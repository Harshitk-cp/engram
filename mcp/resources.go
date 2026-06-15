package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// RegisterResources registers all Engram MCP resources on the server.
func RegisterResources(s *Server, client *Client) {
	s.AddResource(
		Resource{
			URI:         "engram://agents/{agent_id}/memories",
			Name:        "Agent hot memories",
			Description: "Hot-tier memories for an agent — the highest-confidence context.",
			MimeType:    "application/json",
		},
		"engram://agents/{agent_id}/memories",
		hotMemoriesResource(client),
	)

	s.AddResource(
		Resource{
			URI:         "engram://agents/{agent_id}/health",
			Name:        "Agent knowledge health",
			Description: "Knowledge-health snapshot for an agent: memory counts by type, average confidence, contradiction count, memories at risk, and uncertainty areas.",
			MimeType:    "application/json",
		},
		"engram://agents/{agent_id}/health",
		healthResource(client),
	)

	s.AddResource(
		Resource{
			URI:         "engram://agents/{agent_id}/calibration",
			Name:        "Agent confidence calibration",
			Description: "Measured calibration of the agent's confidence scores (ECE / MCE / Brier + reliability diagram).",
			MimeType:    "application/json",
		},
		"engram://agents/{agent_id}/calibration",
		calibrationResource(client),
	)

	s.AddResource(
		Resource{
			URI:         "engram://audit/integrity",
			Name:        "Audit trail integrity",
			Description: "Tamper-evidence check of the tenant's memory audit trail (hash-chain verification result).",
			MimeType:    "application/json",
		},
		"engram://audit/integrity",
		auditIntegrityResource(client),
	)
}

func hotMemoriesResource(client *Client) ResourceHandler {
	return func(ctx context.Context, uri string) (*ResourceContent, error) {
		agentID := extractAgentID(uri)
		if agentID == "" {
			return nil, fmt.Errorf("invalid URI: %s", uri)
		}

		memories, err := client.GetHotMemories(ctx, agentID, 20)
		if err != nil {
			return nil, err
		}

		data, err := json.MarshalIndent(memories, "", "  ")
		if err != nil {
			return nil, err
		}

		return &ResourceContent{
			URI:      uri,
			MimeType: "application/json",
			Text:     string(data),
		}, nil
	}
}

func healthResource(client *Client) ResourceHandler {
	return func(ctx context.Context, uri string) (*ResourceContent, error) {
		agentID := extractAgentID(uri)
		if agentID == "" {
			return nil, fmt.Errorf("invalid URI: %s", uri)
		}
		raw, err := client.GetRaw(ctx, "/v1/cognitive/health?agent_id="+url.QueryEscape(agentID))
		if err != nil {
			return nil, err
		}
		return &ResourceContent{URI: uri, MimeType: "application/json", Text: pretty(raw)}, nil
	}
}

func calibrationResource(client *Client) ResourceHandler {
	return func(ctx context.Context, uri string) (*ResourceContent, error) {
		agentID := extractAgentID(uri)
		if agentID == "" {
			return nil, fmt.Errorf("invalid URI: %s", uri)
		}
		raw, err := client.GetRaw(ctx, "/v1/cognitive/calibration?agent_id="+url.QueryEscape(agentID))
		if err != nil {
			return nil, err
		}
		return &ResourceContent{URI: uri, MimeType: "application/json", Text: pretty(raw)}, nil
	}
}

func auditIntegrityResource(client *Client) ResourceHandler {
	return func(ctx context.Context, uri string) (*ResourceContent, error) {
		raw, err := client.GetRaw(ctx, "/v1/audit/verify")
		if err != nil {
			return nil, err
		}
		return &ResourceContent{URI: uri, MimeType: "application/json", Text: pretty(raw)}, nil
	}
}

// extractAgentID parses the agent ID from a URI like "engram://agents/{agent_id}/...".
func extractAgentID(uri string) string {
	const prefix = "engram://agents/"
	if !strings.HasPrefix(uri, prefix) {
		return ""
	}
	rest := uri[len(prefix):]
	if i := strings.Index(rest, "/"); i >= 0 {
		return rest[:i]
	}
	return rest
}
