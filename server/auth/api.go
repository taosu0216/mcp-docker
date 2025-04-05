package auth

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

func Auth(ctx context.Context, request mcp.CallToolRequest) (context.Context, mcp.CallToolRequest, bool) {
	apiKey := request.Params.Arguments["api_key"].(string)
	if apiKey == "" || apiKey != "123456" {
		return ctx, request, false
	}
	return ctx, request, true
}
