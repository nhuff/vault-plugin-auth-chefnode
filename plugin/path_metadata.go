package chefnode

import (
	"context"

	"github.com/hashicorp/vault/logical"
	"github.com/hashicorp/vault/logical/framework"
)

func pathMetadataList(b *backend) *framework.Path {
	return &framework.Path{
		Pattern: "metadata/rules/?$",
		Callbacks: map[logical.Operation]framework.OperationFunc{
			logical.ListOperation: b.pathMetaRulesList,
		},
		HelpSynopsis:    pathMetadataHelpSyn,
		HelpDescription: pathMetadataHelpDesc,
	}
}

func pathMetadata(b *backend) *framework.Path {
	return &framework.Path{
		Pattern: `metadata/rule/(?P<name>.+)`,
		Fields: map[string]*framework.FieldSchema{
			"name": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "Name of Rule",
			},
			"key": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "Name of the metadata field to populate",
			},
			"query": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "A gjson query to get the desired field",
			},
		},
		Callbacks: map[logical.Operation]framework.OperationFunc{
			logical.DeleteOperation: b.pathMetaRuleDelete,
			logical.ReadOperation:   b.pathMetaRuleRead,
			logical.UpdateOperation: b.pathMetaRuleWrite,
		},
	}
}

func (b *backend) Metadata(ctx context.Context, s logical.Storage, n string) (*MetadataEntry, error) {
	entry, err := s.Get(ctx, "metadata/"+n)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}

	var result MetadataEntry
	if err := entry.DecodeJSON(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (b *backend) pathMetaRulesList(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	metarules, err := req.Storage.List(ctx, "metadata/")
	if err != nil {
		return nil, err
	}
	return logical.ListResponse(metarules), nil
}

func (b *backend) pathMetaRuleDelete(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	err := req.Storage.Delete(ctx, "metadata/"+d.Get("name").(string))
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (b *backend) pathMetaRuleRead(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	metadata, err := b.Metadata(ctx, req.Storage, d.Get("name").(string))
	if err != nil {
		return nil, err
	}
	if metadata == nil {
		return nil, nil
	}

	return &logical.Response{
		Data: map[string]interface{}{
			"key":   metadata.Key,
			"query": metadata.Query,
		},
	}, nil
}

func (b *backend) pathMetaRuleWrite(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	entry, err := logical.StorageEntryJSON("metadata/"+d.Get("name").(string), &MetadataEntry{
		Key:   d.Get("key").(string),
		Query: d.Get("query").(string),
	})
	if err != nil {
		return nil, err
	}
	if err := req.Storage.Put(ctx, entry); err != nil {
		return nil, err
	}

	return nil, nil
}

type MetadataEntry struct {
	Key   string
	Query string
}

const pathMetadataHelpSyn = `
Manage Vault metadata mapping from a Chef attribute
`
const pathMetadataHelpDesc = `
This endpoint allows you to create, read, update, and delete configuration for metadata
mapping from Chef attributes
`
