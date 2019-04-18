package chefnode

import (
	"context"
	"fmt"

	"net/url"

	"github.com/fatih/structs"
	"github.com/hashicorp/vault/helper/policyutil"
	"github.com/hashicorp/vault/logical"
	"github.com/hashicorp/vault/logical/framework"
)

func pathConfig(b *backend) *framework.Path {
	return &framework.Path{
		Pattern: "config",
		Fields: map[string]*framework.FieldSchema{
			"base_url": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: `The URL to the chef server api endpoint`,
			},
			"client_name": &framework.FieldSchema{
				Type: framework.TypeString,
				Description: `Name of the client to connect to chef server with. This needs
to be precreated in the chef server.`,
			},
			"client_key": &framework.FieldSchema{
				Type: framework.TypeString,
				Description: `PEM encoded client key to use for authenticating to chef
server. This is generated when the client is created in the chef server`,
			},
			"default_policies": &framework.FieldSchema{
				Type: framework.TypeString,
				Description: `Comma seperated list of policies given to all tokens that
successfully authenticate against this backend.`,
			},
			"skip_ssl_verify": &framework.FieldSchema{
				Type:        framework.TypeBool,
				Description: `Skip verification of the Chef server SSL certificate.`,
				Default:     false,
			},
		},
		Callbacks: map[logical.Operation]framework.OperationFunc{
			logical.ReadOperation:   b.pathConfigRead,
			logical.UpdateOperation: b.pathConfigWrite,
		},
		HelpSynopsis:    pathConfigHelpSyn,
		HelpDescription: pathConfigHelpDesc,
	}
}

func (b *backend) pathConfigRead(ctx context.Context, req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	cfg, err := b.Config(ctx, req.Storage)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}

	resp := &logical.Response{
		Data: structs.New(cfg).Map(),
	}
	resp.AddWarning("Read access to this endpoint should be controlled via ACLs as it will return the configuration information as-is, including any passwords.")
	return resp, nil
}

func (b *backend) pathConfigWrite(ctx context.Context, req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	baseURL := data.Get("base_url").(string)
	clientName := data.Get("client_name").(string)
	clientKey := data.Get("client_key").(string)
	defaultPolicies := policyutil.ParsePolicies(data.Get("default_policies").(string))
	skipSSL := data.Get("skip_ssl_verify").(bool)

	_, err := parsePrivateKey(clientKey)
	if err != nil {
		return nil, err
	}

	_, err = url.ParseRequestURI(baseURL)
	if err != nil {
		return nil, err
	}

	entry, err := logical.StorageEntryJSON("config", config{
		BaseURL:         baseURL,
		ClientName:      clientName,
		ClientKey:       clientKey,
		DefaultPolicies: defaultPolicies,
		SkipSSL:         skipSSL,
	})

	if err != nil {
		return nil, err
	}

	if err := req.Storage.Put(ctx, entry); err != nil {
		return nil, err
	}

	return nil, nil
}

func (b *backend) Config(ctx context.Context, s logical.Storage) (*config, error) {
	entry, err := s.Get(ctx, "config")
	if err != nil {
		return nil, err
	}

	var result config
	if entry != nil {
		if err := entry.DecodeJSON(&result); err != nil {
			return nil, fmt.Errorf("error reading configuration: %s", err)
		}
	}

	return &result, nil
}

type config struct {
	BaseURL         string   `json:"base_url" structs:"base_url"`
	ClientKey       string   `json:"client_key" structs:"client_key"`
	ClientName      string   `json:"client_name" structs:"client_name"`
	DefaultPolicies []string `json:"default_policies" structs:"default_policies"`
	SkipSSL         bool     `json:"skip_ssl_verify" structs:"skip_ssl_verify"`
}

const pathConfigHelpSyn = `
Configure Vault to connection to Chef server.
`
const pathConfigHelpDesc = `
Configure the URL of the chef server API endpoint and the client name and key used to
make API requests to it.  The client must be already created in the chef server.
Optionally add a default set of policies all clients authenticating against this endpoint
will receive.
`
