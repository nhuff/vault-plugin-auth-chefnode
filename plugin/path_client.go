package chefnode

import (
	"context"
	"github.com/hashicorp/vault/helper/policyutil"
	"github.com/hashicorp/vault/logical"
	"github.com/hashicorp/vault/logical/framework"
)

func pathClientsList(b *backend) *framework.Path {
	return &framework.Path{
		Pattern: "clients/?$",
		Callbacks: map[logical.Operation]framework.OperationFunc{
			logical.ListOperation: b.pathClientList,
		},
		HelpSynopsis:    pathClientHelpSyn,
		HelpDescription: pathClientHelpDesc,
	}
}

func pathClients(b *backend) *framework.Path {
	return &framework.Path{
		Pattern: `client/(?P<name>.+)`,
		Fields: map[string]*framework.FieldSchema{
			"name": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "Name of the Chef client",
			},
			"policies": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "Comma-seperated list of policies associated to this Chef client",
			},
		},
		Callbacks: map[logical.Operation]framework.OperationFunc{
			logical.DeleteOperation: b.pathClientDelete,
			logical.ReadOperation:   b.pathClientRead,
			logical.UpdateOperation: b.pathClientWrite,
		},
	}
}

func (b *backend) pathClientList(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	clients, err := req.Storage.List(ctx, "client/")
	if err != nil {
		return nil, err
	}
	return logical.ListResponse(clients), nil
}

func (b *backend) Client(ctx context.Context, s logical.Storage, n string) (*ClientEntry, error) {
	entry, err := s.Get(ctx, "client/"+n)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}

	var result ClientEntry
	if err := entry.DecodeJSON(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (b *backend) pathClientDelete(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	err := req.Storage.Delete(ctx, "client/"+d.Get("name").(string))
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (b *backend) pathClientRead(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	client, err := b.Client(ctx, req.Storage, d.Get("name").(string))
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, nil
	}

	return &logical.Response{
		Data: map[string]interface{}{
			"policies": client.Policies,
		},
	}, nil
}

func (b *backend) pathClientWrite(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	entry, err := logical.StorageEntryJSON("client/"+d.Get("name").(string), &ClientEntry{
		Policies: policyutil.ParsePolicies(d.Get("policies").(string)),
	})
	if err != nil {
		return nil, err
	}
	if err := req.Storage.Put(ctx, entry); err != nil {
		return nil, err
	}

	return nil, nil
}

type ClientEntry struct {
	Policies []string
}

const pathClientHelpSyn = `
Manage Vault policies assigned to a Chef client
`
const pathClientHelpDesc = `
This endpoint allows you to create, read, update, and delete configuration for policies
associated with Chef clients
`
