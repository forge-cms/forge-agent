package agent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
)

const maxBodyBytes = 32 * 1024 // 32 KB cap on response bodies

// builtinTool pairs an Anthropic tool definition with its Go handler.
type builtinTool struct {
	param   anthropic.ToolUnionParam
	handler func(ctx context.Context, input map[string]any) (string, error)
}

// builtins is the shared HTTP client used by built-in tools.
var builtins = &http.Client{Timeout: 30 * time.Second}

// builtinTools returns the two always-available tools: http_get and http_post.
func builtinTools() []builtinTool {
	return []builtinTool{
		{
			param: anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
				Name:        "http_get",
				Description: anthropic.String("Make an HTTP GET request and return the response body."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]any{
						"url": map[string]any{
							"type":        "string",
							"description": "The URL to fetch.",
						},
					},
					Required: []string{"url"},
				},
			}},
			handler: httpGet,
		},
		{
			param: anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
				Name:        "http_post",
				Description: anthropic.String("Make an HTTP POST request and return the response status and body."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]any{
						"url": map[string]any{
							"type":        "string",
							"description": "The URL to POST to.",
						},
						"body": map[string]any{
							"type":        "string",
							"description": "The request body.",
						},
						"content_type": map[string]any{
							"type":        "string",
							"description": `Content-Type header value. Defaults to "text/plain". Use "application/json" for JSON APIs and Discord webhooks.`,
						},
					},
					Required: []string{"url", "body"},
				},
			}},
			handler: httpPost,
		},
	}
}

func httpGet(ctx context.Context, input map[string]any) (string, error) {
	url, _ := input["url"].(string)
	if url == "" {
		return "", fmt.Errorf("url is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := builtins.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Sprintf("HTTP %d: %s", resp.StatusCode, clip(string(body), 512)), nil
	}
	return string(body), nil
}

func httpPost(ctx context.Context, input map[string]any) (string, error) {
	url, _ := input["url"].(string)
	if url == "" {
		return "", fmt.Errorf("url is required")
	}
	body, _ := input["body"].(string)
	contentType, _ := input["content_type"].(string)
	if contentType == "" {
		contentType = "text/plain"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := builtins.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("HTTP %d: %s", resp.StatusCode, clip(string(respBody), 512)), nil
}

// clip truncates s to at most n bytes, appending "..." if truncated.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
