package chefnode

import (
	"context"

	"github.com/hashicorp/vault/helper/policyutil"
	"github.com/hashicorp/vault/logical"
	"github.com/hashicorp/vault/logical/framework"
)

func pathTagsList(b *backend) *framework.Path {
	return &framework.Path{
		Pattern: "tags/?$",
		Callbacks: map[logical.Operation]framework.OperationFunc{
			logical.ListOperation: b.pathTagsList,
		},
		HelpSynopsis:    pathTagsHelpSyn,
		HelpDescription: pathTagsHelpDesc,
	}
}

func pathTags(b *backend) *framework.Path {
	return &framework.Path{
		Pattern: `tag/(?P<name>.+)`,
		Fields: map[string]*framework.FieldSchema{
			"name": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "Name of the Chef Tag",
			},
			"policies": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "Comma-seperated list of policies associated to this Chef Tag",
			},
		},
		Callbacks: map[logical.Operation]framework.OperationFunc{
			logical.DeleteOperation: b.pathTagDelete,
			logical.ReadOperation:   b.pathTagRead,
			logical.UpdateOperation: b.pathTagWrite,
		},
	}
}

func (b *backend) pathTagsList(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	b.Logger().Info("Listing Tags")
	tags, err := req.Storage.List(ctx, "tag/")
	b.Logger().Info("result", tags, err)
	if err != nil {
		return nil, err
	}
	return logical.ListResponse(tags), nil
}

func (b *backend) Tag(ctx context.Context, s logical.Storage, n string) (*TagEntry, error) {
	entry, err := s.Get(ctx, "tag/"+n)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}

	var result TagEntry
	if err := entry.DecodeJSON(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (b *backend) pathTagDelete(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	err := req.Storage.Delete(ctx, "tag/"+d.Get("name").(string))
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (b *backend) pathTagRead(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	tag, err := b.Tag(ctx, req.Storage, d.Get("name").(string))
	if err != nil {
		return nil, err
	}
	if tag == nil {
		return nil, nil
	}

	return &logical.Response{
		Data: map[string]interface{}{
			"policies": tag.Policies,
		},
	}, nil
}

func (b *backend) pathTagWrite(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	entry, err := logical.StorageEntryJSON("tag/"+d.Get("name").(string), &TagEntry{
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

type TagEntry struct {
	Policies []string
}

const pathTagsHelpSyn = `
Manage Vault policies assigned to a Chef Tag
`
const pathTagsHelpDesc = `
This endpoint allows you to create, read, update, and delete configuration for policies
associated with Chef Tags
`
