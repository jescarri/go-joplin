package mcp

import (
	"context"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const capabilitiesURI = "joplingo://capabilities"

// RegisterResources registers MCP resources (e.g. capabilities document).
func RegisterResources(server *Server, d *Deps) {
	server.AddResource(&sdkmcp.Resource{
		URI:         capabilitiesURI,
		Name:        "capabilities",
		Title:       "Mutation capabilities",
		Description: "JSON document describing which folders/tags are read-write vs read-only, and whether tag/folder creation is allowed. Use this to know which folders you can create notes in.",
		MIMEType:    "application/json",
	}, capabilitiesResourceHandler(d))
}

func capabilitiesResourceHandler(d *Deps) sdkmcp.ResourceHandler {
	return func(ctx context.Context, req *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
		policy := d.Policy
		if policy == nil {
			policy = NewPolicy(nil) // empty policy = all read-only
		}
		raw, err := policy.CapabilitiesJSON(d.DB)
		if err != nil {
			return nil, err
		}
		return &sdkmcp.ReadResourceResult{
			Contents: []*sdkmcp.ResourceContents{
				{URI: capabilitiesURI, MIMEType: "application/json", Text: string(raw)},
			},
		}, nil
	}
}
