package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// RegisterTools registers all Engram MCP tools on the server.
func RegisterTools(s *Server, client *Client) {
	s.AddTool(rememberTool(), rememberHandler(client))
	s.AddTool(recallTool(), recallHandler(client))
	s.AddTool(recallGraphTool(), recallGraphHandler(client))
	s.AddTool(forgetTool(), forgetHandler(client))
	s.AddTool(getHotContextTool(), getHotContextHandler(client))
	s.AddTool(listAgentsTool(), listAgentsHandler(client))
}

// ─── Tool definitions ──────────────────────────────────────────────────────────

func rememberTool() Tool {
	return Tool{
		Name:        "remember",
		Description: "Store a memory in Engram. Use this to persist facts, preferences, decisions, or constraints about a user or context so they can be recalled later.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"content": map[string]interface{}{
					"type":        "string",
					"description": "The content to remember. Be specific and self-contained — this will be retrieved by semantic similarity.",
				},
				"type": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"fact", "preference", "decision", "constraint"},
					"description": "Memory type. Defaults to 'fact'.",
				},
				"source": map[string]interface{}{
					"type": "string",
					"enum": []string{"user", "agent", "inferred", "tool", "derived"},
					"description": "Who this belief ORIGINATED from — set it deliberately, it sets the initial confidence: " +
						"'user' = the user explicitly stated or confirmed it (highest trust, 0.9); " +
						"'inferred' = you inferred it from the conversation, not stated outright (0.4); " +
						"'agent' = the assistant's own reasoning/decision (0.6); " +
						"'tool' = it came from a tool/function result (0.8). " +
						"Use 'user' for facts and preferences the user told you; do not default everything to 'agent'.",
				},
				"agent_id": map[string]interface{}{
					"type":        "string",
					"description": "Agent ID to store the memory for. Uses the default agent if omitted.",
				},
				"anchor": map[string]interface{}{
					"type":        "string",
					"description": "Who/what this memory is about — the caller's own id for a subject (e.g. a customer/guest/patient id). Binds the trace to that anchor so it can be recalled per-subject. Auto-created on first use. Omit for the agent's own general memory.",
				},
				"session_id": map[string]interface{}{
					"type":        "string",
					"description": "Bind this memory to a conversation/session (UUID from POST /v1/sessions). Short-term: it decays after the session ends, and is promoted to the anchor's durable profile if it recurs.",
				},
				"confidence": map[string]interface{}{
					"type": "number",
					"description": "How certain this memory is (0.0–1.0). " +
						"You must reason about this before storing: " +
						"0.85–1.0 = user directly stated or confirmed it; " +
						"0.50–0.85 = inferred from context, probable but not explicit; " +
						"0.20–0.50 = speculative, hedged ('might', 'thinks', 'possibly'); " +
						"omit only when the server should use its own provenance-based default.",
					"minimum": 0,
					"maximum": 1,
				},
			},
			"required": []string{"content"},
		},
	}
}

func recallTool() Tool {
	return Tool{
		Name:        "recall",
		Description: "Retrieve memories from Engram that are semantically similar to a query. Returns the most relevant memories ranked by similarity and confidence.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Natural language query to find relevant memories.",
				},
				"top_k": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of memories to return. Defaults to 10.",
					"minimum":     1,
					"maximum":     50,
				},
				"agent_id": map[string]interface{}{
					"type":        "string",
					"description": "Agent ID to recall from. Uses the default agent if omitted.",
				},
				"anchor": map[string]interface{}{
					"type":        "string",
					"description": "Restrict recall to memories about this subject — the caller's own id for an anchor (e.g. a customer/guest/patient id). Omit to recall the agent's own general memory.",
				},
				"session_id": map[string]interface{}{
					"type":        "string",
					"description": "Fold a session's short-term memory into recall (UUID). Composes the subject's durable profile + this conversation's recent context.",
				},
			},
			"required": []string{"query"},
		},
	}
}

func recallGraphTool() Tool {
	return Tool{
		Name:        "recall_graph",
		Description: "Recall memories using hybrid vector + knowledge graph search. Graph traversal surfaces related memories that pure embedding search might miss.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Natural language query to find relevant memories.",
				},
				"top_k": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of memories to return. Defaults to 10.",
					"minimum":     1,
					"maximum":     50,
				},
				"graph_weight": map[string]interface{}{
					"type":        "number",
					"description": "Weight of graph search vs. vector search (0–1). Defaults to 0.4.",
					"minimum":     0,
					"maximum":     1,
				},
				"agent_id": map[string]interface{}{
					"type":        "string",
					"description": "Agent ID to recall from. Uses the default agent if omitted.",
				},
				"anchor": map[string]interface{}{
					"type":        "string",
					"description": "Restrict recall to memories about this subject — the caller's own id for an anchor (e.g. a customer/guest/patient id). Omit to recall the agent's own general memory.",
				},
				"session_id": map[string]interface{}{
					"type":        "string",
					"description": "Fold a session's short-term memory into recall (UUID). Composes the subject's durable profile + this conversation's recent context.",
				},
			},
			"required": []string{"query"},
		},
	}
}

func forgetTool() Tool {
	return Tool{
		Name:        "forget",
		Description: "Delete a memory from Engram by its ID. Use this to remove outdated or incorrect information.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"memory_id": map[string]interface{}{
					"type":        "string",
					"description": "The UUID of the memory to delete.",
				},
			},
			"required": []string{"memory_id"},
		},
	}
}

func getHotContextTool() Tool {
	return Tool{
		Name:        "get_hot_context",
		Description: "Get the highest-confidence (hot-tier) memories for an agent, formatted for injection into a system prompt. Call this at the start of a session to load relevant context.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"agent_id": map[string]interface{}{
					"type":        "string",
					"description": "Agent ID to retrieve context for. Uses the default agent if omitted.",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of hot memories to include. Defaults to 10.",
					"minimum":     1,
					"maximum":     50,
				},
			},
		},
	}
}

func listAgentsTool() Tool {
	return Tool{
		Name:        "list_agents",
		Description: "List all agents registered under your Engram tenant.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}
}

// ─── Tool handlers ─────────────────────────────────────────────────────────────

func rememberHandler(client *Client) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		content, _ := args["content"].(string)
		if content == "" {
			return ErrorResult("content is required"), nil
		}
		memType, _ := args["type"].(string)
		source, _ := args["source"].(string)
		if source == "" {
			source = "agent"
		}
		agentID, _ := args["agent_id"].(string)
		anchor, _ := args["anchor"].(string)
		session, _ := args["session_id"].(string)

		confidence := floatArg(args, "confidence", 0)

		result, err := client.Remember(ctx, agentID, content, memType, source, anchor, session, float32(confidence))
		if err != nil {
			return ErrorResult(err.Error()), nil
		}

		text := fmt.Sprintf("Stored memory %s\nContent: %s\nTier: %s (confidence %.2f)\n%s",
			result.ID, result.Content, result.Tier, result.Confidence, result.TierReason)
		if result.Reinforced {
			text += "\nNote: existing memory was reinforced"
		}
		return TextResult(text), nil
	}
}

func recallHandler(client *Client) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		query, _ := args["query"].(string)
		if query == "" {
			return ErrorResult("query is required"), nil
		}
		topK := intArg(args, "top_k", 10)
		agentID, _ := args["agent_id"].(string)
		anchor, _ := args["anchor"].(string)
		session, _ := args["session_id"].(string)

		memories, err := client.Recall(ctx, agentID, query, topK, 0, anchor, session)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		return TextResult(formatMemories(memories)), nil
	}
}

func recallGraphHandler(client *Client) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		query, _ := args["query"].(string)
		if query == "" {
			return ErrorResult("query is required"), nil
		}
		topK := intArg(args, "top_k", 10)
		graphWeight := floatArg(args, "graph_weight", 0.4)
		agentID, _ := args["agent_id"].(string)
		anchor, _ := args["anchor"].(string)
		session, _ := args["session_id"].(string)

		memories, err := client.Recall(ctx, agentID, query, topK, graphWeight, anchor, session)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}
		return TextResult(formatMemories(memories)), nil
	}
}

func forgetHandler(client *Client) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		memoryID, _ := args["memory_id"].(string)
		if memoryID == "" {
			return ErrorResult("memory_id is required"), nil
		}

		if err := client.Forget(ctx, memoryID); err != nil {
			return ErrorResult(err.Error()), nil
		}
		return TextResult(fmt.Sprintf("Memory %s deleted.", memoryID)), nil
	}
}

func getHotContextHandler(client *Client) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		agentID, _ := args["agent_id"].(string)
		limit := intArg(args, "limit", 10)

		memories, err := client.GetHotMemories(ctx, agentID, limit)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}

		if len(memories) == 0 {
			return TextResult("No hot-tier memories found. The agent has no high-confidence context yet."), nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Known context (%d high-confidence memories):\n", len(memories)))
		for i, m := range memories {
			sb.WriteString(fmt.Sprintf("%d. [%s, confidence %.2f] %s\n", i+1, m.Type, m.Confidence, m.Content))
		}
		return TextResult(sb.String()), nil
	}
}

func listAgentsHandler(client *Client) ToolHandler {
	return func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
		agents, err := client.ListAgents(ctx)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}

		if len(agents) == 0 {
			return TextResult("No agents found."), nil
		}

		data, _ := json.MarshalIndent(agents, "", "  ")
		return TextResult(string(data)), nil
	}
}

// ─── Helpers ───────────────────────────────────────────────────────────────────

func formatMemories(memories []RecallMemory) string {
	if len(memories) == 0 {
		return "No memories found matching the query."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d memories:\n\n", len(memories)))
	for i, m := range memories {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n   ID: %s | Confidence: %.2f | Score: %.3f\n\n",
			i+1, m.Type, m.Content, m.ID, m.Confidence, m.Score))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func intArg(args map[string]interface{}, key string, defaultVal int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return defaultVal
}

func floatArg(args map[string]interface{}, key string, defaultVal float64) float64 {
	if v, ok := args[key].(float64); ok {
		return v
	}
	return defaultVal
}
