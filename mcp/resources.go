package mcp

import (
	"context"
	"encoding/json"
	"fmt"
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
			Name:        "Agent health",
			Description: "Memory health snapshot for an agent — tier distribution and recent mutations.",
			MimeType:    "text/plain",
		},
		"engram://agents/{agent_id}/health",
		healthResource(client),
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

		// Use hot memories as a proxy for health until the dedicated health endpoint lands (P2-1).
		memories, err := client.GetHotMemories(ctx, agentID, 50)
		if err != nil {
			return nil, err
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Agent: %s\n", agentID))
		sb.WriteString(fmt.Sprintf("Hot-tier memories: %d\n\n", len(memories)))

		if len(memories) > 0 {
			sb.WriteString("Top memories by confidence:\n")
			limit := len(memories)
			if limit > 10 {
				limit = 10
			}
			for i, m := range memories[:limit] {
				sb.WriteString(fmt.Sprintf("  %d. [%.2f] %s\n", i+1, m.Confidence, m.Content))
			}
		}

		return &ResourceContent{
			URI:      uri,
			MimeType: "text/plain",
			Text:     sb.String(),
		}, nil
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
