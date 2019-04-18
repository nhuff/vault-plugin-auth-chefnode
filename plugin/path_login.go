package chefnode

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strings"

	"time"

	chefapi "github.com/go-chef/chef"
	"github.com/hashicorp/vault/helper/policyutil"
	"github.com/hashicorp/vault/helper/strutil"
	"github.com/hashicorp/vault/logical"
	"github.com/hashicorp/vault/logical/framework"
	"github.com/tidwall/gjson"
)

func pathLogin(b *backend) *framework.Path {
	return &framework.Path{
		Pattern: "login",
		Fields: map[string]*framework.FieldSchema{
			"signature_version": &framework.FieldSchema{
				Type: framework.TypeString,
				Description: `Version of the Chef signature algorithm to use. Corresponds
to the value that mixlib-authentication will set for the X-Ops-Sign HTTP header.
Currently only version 1.0 of the signature algorithm is supported so this should
be set to 'algorithm=sha1;version=1.0;'`,
			},
			"client_name": &framework.FieldSchema{
				Type: framework.TypeString,
				Description: `The name of the client to authenticate. Corresponds
to the value of the X-Ops-UserId header set by mixlib-authentication`,
			},
			"timestamp": &framework.FieldSchema{
				Type: framework.TypeString,
				Description: `Timestamp of signature in time.RFC3339 format. Corresponds
to X-Ops-Timestamp header returned by mixlib-authentcation. This backend currently checks
that the timestamp is within 5 minutes of vaults current time.`,
			},
			"signature": &framework.FieldSchema{
				Type: framework.TypeString,
				Description: `Signature of authentication request. Corresponds to the
X-Ops-Authorization-* headers returned by mixlib-authentication. The value should be given
as one value rather than the split value generated by mixlib-authentication.`,
			},
		},
		Callbacks: map[logical.Operation]framework.OperationFunc{
			logical.UpdateOperation: b.pathLogin,
		},
		HelpSynopsis:    pathLoginSyn,
		HelpDescription: pathLoginDesc,
	}
}

func (b *backend) pathLogin(ctx context.Context, req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	client := data.Get("client_name").(string)
	ts := data.Get("timestamp").(string)
	sig := data.Get("signature").(string)
	sigVer := data.Get("signature_version").(string)

	keys, err := b.retrievePubKey(ctx, req, client)
	if err != nil {
		return nil, err
	}
	reqPath := "/v1/" + req.MountPoint + req.Path
	auth := authenticate(client, ts, sig, sigVer, keys, reqPath)
	if !auth {
		return logical.ErrorResponse("Couldn't authenticate client"), nil
	}

	allowedSkew := time.Minute * 5
	now := time.Now().UTC()
	headerTime, err := time.Parse(time.RFC3339, data.Get("timestamp").(string))
	if err != nil {
		return nil, err
	}

	if math.Abs(float64(now.Sub(headerTime))) > float64(allowedSkew) {
		return nil, fmt.Errorf("clock skew is too great for request")
	}

	policies, err := b.getNodePolicies(ctx, req, client)
	if err != nil {
		return nil, err
	}

	metadata, err := b.getNodeMetadata(ctx, req, client)
	if err != nil {
		return nil, err
	}
	// pre-create the alias
	alias := &logical.Alias{
		Name:     client,
		Metadata: metadata,
	}

	// get or create the entity
	return &logical.Response{
		Auth: &logical.Auth{
			Policies:    policies,
			DisplayName: client,
			LeaseOptions: logical.LeaseOptions{
				Renewable: true,
			},
			InternalData: map[string]interface{}{
				"request_path":      reqPath,
				"signature_version": data.Get("signature_version"),
				"signature":         data.Get("signature"),
				"client_name":       data.Get("client_name"),
				"timestamp":         data.Get("timestamp"),
			},
			Alias: alias,
		},
	}, nil
}

func (b *backend) pathLoginRenew(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	if req.Auth == nil {
		return nil, fmt.Errorf("request auth was nil")
	}

	reqPath := req.Auth.InternalData["request_path"].(string)
	sig := req.Auth.InternalData["signature"].(string)
	sigVer := req.Auth.InternalData["signature_version"].(string)
	client := req.Auth.InternalData["client_name"].(string)
	ts := req.Auth.InternalData["timestamp"].(string)

	keys, err := b.retrievePubKey(ctx, req, client)
	if err != nil {
		return nil, err
	}

	auth := authenticate(client, ts, sig, sigVer, keys, reqPath)
	if !auth {
		return nil, fmt.Errorf("couldn't authenticate renew request")
	}

	policies, err := b.getNodePolicies(ctx, req, client)
	if err != nil {
		return nil, fmt.Errorf("coulnd't retrieve current policy list")
	}

	if !policyutil.EquivalentPolicies(policies, req.Auth.Policies) {
		return nil, fmt.Errorf("policies have changed, not renewing")
	}

	metadata, err := b.getNodeMetadata(ctx, req, client)
	if err != nil {
		return nil, err
	}

	if !reflect.DeepEqual(metadata, req.Auth.Metadata) {
		return nil, fmt.Errorf("metadata have changed, not renewing")
	}

	return framework.LeaseExtend(0, 0, b.System())(ctx, req, d)
}

func (b *backend) getNodePolicies(ctx context.Context, req *logical.Request, node string) ([]string, error) {
	var clientPols []string
	clientEntry, err := b.Client(ctx, req.Storage, node)
	if err != nil {
		return nil, err
	}
	if clientEntry != nil {
		clientPols = clientEntry.Policies
	}
	config, err := b.Config(ctx, req.Storage)
	if err != nil {
		return nil, err
	}
	defaultPols := config.DefaultPolicies

	chefclient, err := b.ChefClient(ctx, req)
	if err != nil {
		return nil, err
	}

	// get the node information from chef
	chefNode, err := chefclient.Nodes.Get(node)
	if err != nil {
		return nil, err
	}

	// environment policies
	var envPols []string
	envEntry, err := b.Environment(ctx, req.Storage, chefNode.Environment)
	if err != nil {
		return nil, err
	}
	if envEntry != nil {
		envPols = envEntry.Policies
	}

	// role policies
	var rolePols []string
	// iterate over the run list to find all roles
	roleRe := regexp.MustCompile(`role\[(.+?)\]`)
	for i := range chefNode.RunList {
		reRes := roleRe.FindStringSubmatch(chefNode.RunList[i])
		if len(reRes) >= 2 {
			// we found a role, check for any policies
			roleEntry, err := b.Role(ctx, req.Storage, reRes[1])
			if err != nil {
				continue
			}
			if roleEntry != nil {
				rolePols = append(rolePols, roleEntry.Policies...)
			}
		}
	}

	// tags
	var tagPols []string
	// this will fail if someone fucks up the chef attributes
	// maybe we should check before we use range on it
	nodeTags := chefNode.NormalAttributes["tags"].([]interface{})
	for i := range nodeTags {
		tagEntry, err := b.Tag(ctx, req.Storage, nodeTags[i].(string))
		if err != nil {
			continue
		}
		if tagEntry != nil {
			tagPols = append(tagPols, tagEntry.Policies...)
		}
	}

	var allPol []string
	allPol = append(allPol, clientPols...)
	allPol = append(allPol, envPols...)
	allPol = append(allPol, rolePols...)
	allPol = append(allPol, tagPols...)
	allPol = append(allPol, defaultPols...)
	allPol = strutil.RemoveDuplicates(allPol, false)

	return allPol, nil
}

func (b *backend) getNodeMetadata(ctx context.Context, req *logical.Request, node string) (map[string]string, error) {
	// check if we have any metadata rules
	metadataRules, err := req.Storage.List(ctx, "metadata/")
	if err != nil {
		return nil, err
	}
	if metadataRules == nil {
		return nil, nil
	}
	if len(metadataRules) == 0 {
		return nil, nil
	}

	// get our chefNode
	chefclient, err := b.ChefClient(ctx, req)
	if err != nil {
		return nil, err
	}

	chefNode, err := chefclient.Nodes.Get(node)
	if err != nil {
		return nil, err
	}

	metadata := make(map[string]string)

	jsonData, err := json.Marshal(chefNode.NormalAttributes)
	if err != nil {
		return nil, err
	}

	json := gjson.Parse(string(jsonData))
	// we iterate over all rules and apply them if they match

	for _, rulename := range metadataRules {
		rule, err := b.Metadata(ctx, req.Storage, rulename)
		if err != nil {
			continue
		}
		result := json.Get(rule.Query)
		if !result.Exists() {
			continue
		}
		if result.IsObject() {
			// if this is an object will fail if there is more then one child
			obj := result.Map()
			keys := reflect.ValueOf(obj).MapKeys()
			if len(keys) != 1 {
				continue
			}
			metadata[rule.Key] = keys[0].String()
		} else if result.IsArray() {
			// we don't support arrays right now
			continue
		} else {
			metadata[rule.Key] = result.String()
		}
	}
	// return the metadata
	return metadata, nil
}

func (b *backend) retrievePubKey(ctx context.Context, req *logical.Request, targetName string) ([]*rsa.PublicKey, error) {
	chefclient, err := b.ChefClient(ctx, req)
	if err != nil {
		return nil, err
	}

	// get the keylist from the client
	clientKeylist, err := chefclient.Clients.ListKeys(targetName)
	if err != nil {
		return nil, err
	}

	// get all keys
	var keys []*rsa.PublicKey
	for i := range *clientKeylist {
		if (*clientKeylist)[i].Expired == false {
			// get the key from the server
			key, err := chefclient.Clients.GetKey(targetName, (*clientKeylist)[i].Name)
			if err != nil {
				continue
			}
			rsaKey, err := parsePublicKey(key.PublicKey)
			if err != nil {
				continue
			}
			keys = append(keys, rsaKey)
		}
	}
	return keys, nil
}

func (b *backend) ChefClient(ctx context.Context, req *logical.Request) (*chefapi.Client, error) {
	// get the config from the backend
	config, err := b.Config(ctx, req.Storage)
	if err != nil {
		return nil, err
	}

	// create a new chef api client
	return chefapi.NewClient(&chefapi.Config{
		Name:    config.ClientName,
		Key:     config.ClientKey,
		BaseURL: config.BaseURL,
		SkipSSL: config.SkipSSL,
	})
}

func authenticate(client string, ts string, sig string, sigVer string, keys []*rsa.PublicKey, path string) bool {
	bodyHash := sha1.Sum([]byte(""))
	hashedPath := sha1.Sum([]byte(path))
	headers := []string{
		"Method:POST",
		"Hashed Path:" + base64.StdEncoding.EncodeToString(hashedPath[:]),
		"X-Ops-Content-Hash:" + base64.StdEncoding.EncodeToString(bodyHash[:]),
		"X-Ops-Timestamp:" + ts,
		"X-Ops-UserId:" + client,
	}
	headerString := strings.Join(headers, "\n")
	decSig, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return false
	}
	for i := range keys {
		err = rsa.VerifyPKCS1v15(keys[i], crypto.Hash(0), []byte(headerString), decSig)
		if err == nil {
			return true
		}
	}
	return false
}

func parsePublicKey(key string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(key))
	if block == nil {
		return nil, fmt.Errorf("Couldn't parse PEM data")
	}
	pubkey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return pubkey.(*rsa.PublicKey), nil
}

type keyInfo struct {
	URI     string `json:"uri"`
	Expired bool   `json:"expired"`
}

type keyResponse struct {
	ClientKey string `json:"public_key"`
}

const pathLoginSyn = `
Authenticate a Chef node to Vault.
`

const pathLoginDesc = `
A Chef node is authenticated against a Chef server using a signature generated using
its Chef client key.
`
