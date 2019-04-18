package chefnode

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/hashicorp/vault/logical"
	"github.com/hashicorp/vault/logical/framework"
)

func Factory(ctx context.Context, conf *logical.BackendConfig) (logical.Backend, error) {
	b := Backend()
	err := b.Setup(ctx, conf)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func Backend() *backend {
	var b backend
	b.Backend = &framework.Backend{
		Help:        backendHelp,
		BackendType: logical.TypeCredential,
		PathsSpecial: &logical.Paths{
			Unauthenticated: []string{
				"login",
			},
		},

		Paths: append([]*framework.Path{
			pathLogin(&b),
			pathConfig(&b),
			pathClients(&b),
			pathClientsList(&b),
			pathEnvironments(&b),
			pathEnvironmentsList(&b),
			pathRoles(&b),
			pathRolesList(&b),
			pathTags(&b),
			pathTagsList(&b),
			pathMetadata(&b),
			pathMetadataList(&b),
		}),

		AuthRenew: b.pathLoginRenew,
	}

	return &b
}

type backend struct {
	*framework.Backend
}

func parsePrivateKey(key string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(key))
	if block == nil {
		return nil, fmt.Errorf("Couldn't parse PEM data")
	}
	privkey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return privkey, nil
}

const backendHelp = `
chef-node authentication backend takes a signature, client name, timestamp,
and signature version such as those generated by the mixlib-authentication
rubygem used by chef to authenticate a chef client against a chef server.
Currently only version 1.0 of the chef signing algorithm is supported.

Policies can be assigned based on chef environment, roles, and tags that are
assigned to the node in chef.  These are configured using the 'environment/<environment>',
'role/<role>', and 'tag/<tag>' endpoints.  The node will get the union of the
policies for the environment, roles, and tags that are applied to it.
`
