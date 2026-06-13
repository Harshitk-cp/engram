package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
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

	// ── memory operations ──
	s.AddTool(getMemoryTool(), getMemoryHandler(client))
	s.AddTool(listMemoriesTool(), listMemoriesHandler(client))
	s.AddTool(explainMemoryTool(), explainMemoryHandler(client))
	s.AddTool(restoreMemoryTool(), restoreMemoryHandler(client))
	s.AddTool(recordFeedbackTool(), recordFeedbackHandler(client))

	// ── conversation ingestion ──
	s.AddTool(ingestConversationTool(), ingestConversationHandler(client))

	// ── episodic & procedural memory ──
	s.AddTool(recallEpisodesTool(), recallEpisodesHandler(client))
	s.AddTool(recordEpisodeTool(), recordEpisodeHandler(client))
	s.AddTool(matchProceduresTool(), matchProceduresHandler(client))

	// ── cognitive & trust ──
	s.AddTool(knowledgeHealthTool(), knowledgeHealthHandler(client))
	s.AddTool(getCalibrationTool(), getCalibrationHandler(client))
	s.AddTool(listContradictionsTool(), listContradictionsHandler(client))
	s.AddTool(verifyAuditTool(), verifyAuditHandler(client))
	s.AddTool(assessConfidenceTool(), assessConfidenceHandler(client))
	s.AddTool(detectUncertaintyTool(), detectUncertaintyHandler(client))
	s.AddTool(reflectTool(), reflectHandler(client))
	s.AddTool(consolidateTool(), consolidateHandler(client))

	// ── multi-subject (anchors) ──
	s.AddTool(createAnchorTool(), createAnchorHandler(client))
	s.AddTool(listAnchorsTool(), listAnchorsHandler(client))
	s.AddTool(forgetSubjectTool(), forgetSubjectHandler(client))

	// ── sessions ──
	s.AddTool(createSessionTool(), createSessionHandler(client))
	s.AddTool(endSessionTool(), endSessionHandler(client))

	// ── working memory, schemas, knowledge graph ──
	s.AddTool(activateContextTool(), activateContextHandler(client))
	s.AddTool(listSchemasTool(), listSchemasHandler(client))
	s.AddTool(matchSchemasTool(), matchSchemasHandler(client))
	s.AddTool(listEntitiesTool(), listEntitiesHandler(client))

	// ── agents & canon ──
	s.AddTool(createAgentTool(), createAgentHandler(client))
	s.AddTool(listCanonTool(), listCanonHandler(client))
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

// ─── schema helpers (keep tool defs compact) ─────────────────────────────────

func obj(props map[string]interface{}, required ...string) map[string]interface{} {
	m := map[string]interface{}{"type": "object", "properties": props}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}
func strF(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "description": desc}
}
func intF(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "integer", "description": desc}
}
func numF(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "number", "description": desc}
}
func enumF(desc string, vals ...string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "enum": vals, "description": desc}
}

func pretty(raw json.RawMessage) string {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(out)
}

func rawResult(raw json.RawMessage, err error) (*CallToolResult, error) {
	if err != nil {
		return ErrorResult(err.Error()), nil
	}
	return TextResult(pretty(raw)), nil
}

// ─── memory operations ───────────────────────────────────────────────────────

func getMemoryTool() Tool {
	return Tool{Name: "get_memory", Description: "Fetch a single memory by its UUID, including its content, type, confidence, tier, and provenance.",
		InputSchema: obj(map[string]interface{}{"memory_id": strF("UUID of the memory.")}, "memory_id")}
}
func getMemoryHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		id, _ := a["memory_id"].(string)
		if id == "" {
			return ErrorResult("memory_id is required"), nil
		}
		return rawResult(c.GetRaw(ctx, "/v1/memories/"+url.PathEscape(id)))
	}
}

func listMemoriesTool() Tool {
	return Tool{Name: "list_memories", Description: "Browse an agent's stored memories (most recent first). Use to audit what the agent knows.",
		InputSchema: obj(map[string]interface{}{
			"agent_id": strF("Agent ID. Uses the default agent if omitted."),
			"limit":    intF("Max memories to return. Defaults to 25."),
		})}
}
func listMemoriesHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		agent := c.ResolveAgent(strArg(a, "agent_id"))
		limit := intArg(a, "limit", 25)
		return rawResult(c.GetRaw(ctx, fmt.Sprintf("/v1/agents/%s/memories?limit=%d", url.PathEscape(agent), limit)))
	}
}

func explainMemoryTool() Tool {
	return Tool{Name: "explain_memory", Description: "Get the full mutation history of a memory — every create/reinforce/decay/contradict/redact event with confidence deltas and reasons. This is the provenance/why-trail: it answers 'why does the agent believe this, and how did its confidence change?'",
		InputSchema: obj(map[string]interface{}{
			"memory_id": strF("UUID of the memory."),
			"limit":     intF("Max history entries. Defaults to 50."),
		}, "memory_id")}
}
func explainMemoryHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		id, _ := a["memory_id"].(string)
		if id == "" {
			return ErrorResult("memory_id is required"), nil
		}
		limit := intArg(a, "limit", 50)
		return rawResult(c.GetRaw(ctx, fmt.Sprintf("/v1/memories/%s/mutations?limit=%d", url.PathEscape(id), limit)))
	}
}

func restoreMemoryTool() Tool {
	return Tool{Name: "restore_memory", Description: "Restore (un-archive) a previously archived/soft-deleted memory by its UUID.",
		InputSchema: obj(map[string]interface{}{"memory_id": strF("UUID of the memory to restore.")}, "memory_id")}
}
func restoreMemoryHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		id, _ := a["memory_id"].(string)
		if id == "" {
			return ErrorResult("memory_id is required"), nil
		}
		return rawResult(c.PostRaw(ctx, "/v1/memories/"+url.PathEscape(id)+"/restore", nil))
	}
}

func recordFeedbackTool() Tool {
	return Tool{Name: "record_feedback", Description: "Record feedback on a memory to drive its belief dynamics. 'helpful'/'used' reinforce it; 'unhelpful'/'contradicted'/'outdated' weaken it; 'ignored' lightly penalizes. Use after you act on (or reject) a recalled memory so confidence stays calibrated.",
		InputSchema: obj(map[string]interface{}{
			"memory_id":   strF("UUID of the memory the feedback is about."),
			"signal_type": enumF("The feedback signal.", "helpful", "unhelpful", "used", "ignored", "contradicted", "outdated"),
			"agent_id":    strF("Agent ID. Uses the default agent if omitted."),
		}, "memory_id", "signal_type")}
}
func recordFeedbackHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		id, _ := a["memory_id"].(string)
		sig, _ := a["signal_type"].(string)
		if id == "" || sig == "" {
			return ErrorResult("memory_id and signal_type are required"), nil
		}
		body := map[string]interface{}{"memory_id": id, "signal_type": sig, "agent_id": c.ResolveAgent(strArg(a, "agent_id"))}
		return rawResult(c.PostRaw(ctx, "/v1/feedback", body))
	}
}

// ─── conversation ingestion ──────────────────────────────────────────────────

func ingestConversationTool() Tool {
	return Tool{Name: "ingest_conversation", Description: "Extract and store memories from a conversation transcript in one call. Engram's LLM pulls out facts, preferences, decisions, and constraints (with provenance/confidence) so you don't have to call remember per fact. Ideal at the end of a session.",
		InputSchema: obj(map[string]interface{}{
			"messages": map[string]interface{}{
				"type":        "array",
				"description": "The conversation turns to extract from.",
				"items": obj(map[string]interface{}{
					"role":    enumF("Who spoke.", "user", "assistant", "system"),
					"content": strF("The message text."),
				}, "role", "content"),
			},
			"agent_id":   strF("Agent ID. Uses the default agent if omitted."),
			"anchor":     strF("Bind extracted memories to this subject (your own customer/guest/patient id). Optional."),
			"session_id": strF("Bind to a conversation/session UUID. Optional."),
			"event_date": strF("ISO-8601 timestamp of when the conversation happened. Optional; defaults to now."),
			"sync":       map[string]interface{}{"type": "boolean", "description": "Block until extraction completes (default true) so the memories are immediately recallable."},
		}, "messages")}
}
func ingestConversationHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		rawMsgs, ok := a["messages"].([]interface{})
		if !ok || len(rawMsgs) == 0 {
			return ErrorResult("messages is required (non-empty array of {role, content})"), nil
		}
		msgs := make([]map[string]interface{}, 0, len(rawMsgs))
		for _, m := range rawMsgs {
			mm, ok := m.(map[string]interface{})
			if !ok {
				return ErrorResult("each message must be an object with role and content"), nil
			}
			msgs = append(msgs, map[string]interface{}{"role": mm["role"], "content": mm["content"]})
		}
		sync := true
		if v, ok := a["sync"].(bool); ok {
			sync = v
		}
		body := map[string]interface{}{"messages": msgs, "sync": sync}
		if v := strArg(a, "anchor"); v != "" {
			body["anchor_external_id"] = v
		}
		if v := strArg(a, "session_id"); v != "" {
			body["session_id"] = v
		}
		if v := strArg(a, "event_date"); v != "" {
			body["event_date"] = v
		}
		agent := c.ResolveAgent(strArg(a, "agent_id"))
		return rawResult(c.PostRaw(ctx, fmt.Sprintf("/v1/agents/%s/conversations/ingest", url.PathEscape(agent)), body))
	}
}

// ─── episodic & procedural ───────────────────────────────────────────────────

func recallEpisodesTool() Tool {
	return Tool{Name: "recall_episodes", Description: "Recall episodic memories — past experiences/interactions with their outcomes — semantically similar to a query.",
		InputSchema: obj(map[string]interface{}{
			"query":    strF("What experience to look for."),
			"agent_id": strF("Agent ID. Uses the default agent if omitted."),
			"limit":    intF("Max episodes. Defaults to 10."),
		}, "query")}
}
func recallEpisodesHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		q, _ := a["query"].(string)
		if q == "" {
			return ErrorResult("query is required"), nil
		}
		p := url.Values{}
		p.Set("agent_id", c.ResolveAgent(strArg(a, "agent_id")))
		p.Set("query", q)
		p.Set("limit", strconv.Itoa(intArg(a, "limit", 10)))
		return rawResult(c.GetRaw(ctx, "/v1/episodes/recall?"+p.Encode()))
	}
}

func recordEpisodeTool() Tool {
	return Tool{Name: "record_episode", Description: "Record an episodic memory — a discrete experience or interaction, optionally with its outcome (success/failure/neutral). Episodes later consolidate into procedures and beliefs.",
		InputSchema: obj(map[string]interface{}{
			"raw_content": strF("Description of what happened."),
			"agent_id":    strF("Agent ID. Uses the default agent if omitted."),
			"outcome":     enumF("How it turned out.", "success", "failure", "neutral", "unknown"),
			"occurred_at": strF("ISO-8601 timestamp. Optional; defaults to now."),
		}, "raw_content")}
}
func recordEpisodeHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		content, _ := a["raw_content"].(string)
		if content == "" {
			return ErrorResult("raw_content is required"), nil
		}
		body := map[string]interface{}{"agent_id": c.ResolveAgent(strArg(a, "agent_id")), "raw_content": content}
		if v := strArg(a, "outcome"); v != "" {
			body["outcome"] = v
		}
		if v := strArg(a, "occurred_at"); v != "" {
			body["occurred_at"] = v
		}
		return rawResult(c.PostRaw(ctx, "/v1/episodes", body))
	}
}

func matchProceduresTool() Tool {
	return Tool{Name: "match_procedures", Description: "Find learned procedures (trigger→action skills) that match the current situation. Use to reuse approaches that worked before.",
		InputSchema: obj(map[string]interface{}{
			"situation": strF("The current situation/trigger to match against learned skills."),
			"agent_id":  strF("Agent ID. Uses the default agent if omitted."),
			"limit":     intF("Max procedures. Defaults to 5."),
		}, "situation")}
}
func matchProceduresHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		sit, _ := a["situation"].(string)
		if sit == "" {
			return ErrorResult("situation is required"), nil
		}
		body := map[string]interface{}{"agent_id": c.ResolveAgent(strArg(a, "agent_id")), "situation": sit, "limit": intArg(a, "limit", 5)}
		return rawResult(c.PostRaw(ctx, "/v1/procedures/match", body))
	}
}

// ─── cognitive & trust ───────────────────────────────────────────────────────

func knowledgeHealthTool() Tool {
	return Tool{Name: "knowledge_health", Description: "Assess the overall health of an agent's knowledge: memory counts by type, average confidence, contradiction count, memories at risk (stale/low-confidence), and uncertainty areas.",
		InputSchema: obj(map[string]interface{}{"agent_id": strF("Agent ID. Uses the default agent if omitted.")})}
}
func knowledgeHealthHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		return rawResult(c.GetRaw(ctx, "/v1/cognitive/health?agent_id="+url.QueryEscape(c.ResolveAgent(strArg(a, "agent_id")))))
	}
}

func getCalibrationTool() Tool {
	return Tool{Name: "get_calibration", Description: "Measure how well the agent's confidence scores are calibrated: ECE, MCE, Brier score, and a reliability diagram, derived from feedback/outcome evidence. Returns 'insufficient' until enough labeled evidence has accrued.",
		InputSchema: obj(map[string]interface{}{"agent_id": strF("Agent ID. Omit for tenant-wide calibration.")})}
}
func getCalibrationHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		path := "/v1/cognitive/calibration"
		if v := strArg(a, "agent_id"); v != "" {
			path += "?agent_id=" + url.QueryEscape(v)
		}
		return rawResult(c.GetRaw(ctx, path))
	}
}

func listContradictionsTool() Tool {
	return Tool{Name: "list_contradictions", Description: "List pairs of the agent's beliefs that conflict with each other, so they can be reviewed or resolved.",
		InputSchema: obj(map[string]interface{}{
			"agent_id": strF("Agent ID. Uses the default agent if omitted."),
			"limit":    intF("Max contradiction pairs. Defaults to 20."),
		})}
}
func listContradictionsHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		agent := c.ResolveAgent(strArg(a, "agent_id"))
		return rawResult(c.GetRaw(ctx, fmt.Sprintf("/v1/agents/%s/contradictions?limit=%d", url.PathEscape(agent), intArg(a, "limit", 20))))
	}
}

func verifyAuditTool() Tool {
	return Tool{Name: "verify_audit", Description: "Verify the tenant's tamper-evident audit trail. Recomputes the per-tenant SHA-256 hash chain over every memory mutation and reports whether it is intact (valid), how many entries were checked, and the chain head. Any edit, insertion, deletion, or reordering of history makes this report invalid.",
		InputSchema: obj(map[string]interface{}{})}
}
func verifyAuditHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		return rawResult(c.GetRaw(ctx, "/v1/audit/verify"))
	}
}

func assessConfidenceTool() Tool {
	return Tool{Name: "assess_confidence", Description: "Get a metacognitive assessment of how much to trust a specific memory: an adjusted confidence accounting for recency, reinforcement, source reliability, and contradictions, with an explanation.",
		InputSchema: obj(map[string]interface{}{"memory_id": strF("UUID of the memory to assess.")}, "memory_id")}
}
func assessConfidenceHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		id, _ := a["memory_id"].(string)
		if id == "" {
			return ErrorResult("memory_id is required"), nil
		}
		return rawResult(c.GetRaw(ctx, "/v1/cognitive/confidence?memory_id="+url.QueryEscape(id)))
	}
}

func detectUncertaintyTool() Tool {
	return Tool{Name: "detect_uncertainty", Description: "Surface the areas where an agent's knowledge is weak or uncertain (low confidence, stale, or conflicting), optionally focused on a topic.",
		InputSchema: obj(map[string]interface{}{
			"agent_id": strF("Agent ID. Uses the default agent if omitted."),
			"topic":    strF("Optional topic to focus the uncertainty scan."),
		})}
}
func detectUncertaintyHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		p := url.Values{}
		p.Set("agent_id", c.ResolveAgent(strArg(a, "agent_id")))
		if v := strArg(a, "topic"); v != "" {
			p.Set("topic", v)
		}
		return rawResult(c.GetRaw(ctx, "/v1/cognitive/uncertainty?"+p.Encode()))
	}
}

func reflectTool() Tool {
	return Tool{Name: "reflect", Description: "Run a metacognitive reflection pass over the agent's knowledge — assessing confidence distribution, uncertainty, and strategy — and return a summary of its knowledge state.",
		InputSchema: obj(map[string]interface{}{
			"agent_id": strF("Agent ID. Uses the default agent if omitted."),
			"focus":    enumF("What to reflect on.", "all", "confidence", "uncertainty", "strategy"),
		})}
}
func reflectHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		body := map[string]interface{}{"agent_id": c.ResolveAgent(strArg(a, "agent_id"))}
		if v := strArg(a, "focus"); v != "" {
			body["focus"] = v
		}
		return rawResult(c.PostRaw(ctx, "/v1/cognitive/reflect", body))
	}
}

func consolidateTool() Tool {
	return Tool{Name: "consolidate", Description: "Trigger the consolidation pipeline for an agent: process raw episodes into semantic beliefs, learn procedures, form schemas, and prune/decay stale memories. Normally runs automatically; call to force it.",
		InputSchema: obj(map[string]interface{}{
			"agent_id": strF("Agent ID. Uses the default agent if omitted."),
			"scope":    enumF("How much to consolidate.", "recent", "full"),
		})}
}
func consolidateHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		body := map[string]interface{}{"agent_id": c.ResolveAgent(strArg(a, "agent_id"))}
		if v := strArg(a, "scope"); v != "" {
			body["scope"] = v
		}
		return rawResult(c.PostRaw(ctx, "/v1/cognitive/consolidate", body))
	}
}

// ─── multi-subject (anchors) ─────────────────────────────────────────────────

func createAnchorTool() Tool {
	return Tool{Name: "create_anchor", Description: "Create an anchor — a subject that memories can be about (a customer, patient, guest, account, etc.). Memories bound to an anchor are isolated per-subject and can be erased per-subject for GDPR.",
		InputSchema: obj(map[string]interface{}{
			"name":        strF("Human-readable name of the subject."),
			"external_id": strF("Your own stable id for this subject (recommended, used to bind memories)."),
			"entity_type": strF("Optional type, e.g. 'customer', 'patient'."),
			"agent_id":    strF("Agent ID. Uses the default agent if omitted."),
		}, "name")}
}
func createAnchorHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		name, _ := a["name"].(string)
		if name == "" {
			return ErrorResult("name is required"), nil
		}
		body := map[string]interface{}{"name": name, "agent_id": c.ResolveAgent(strArg(a, "agent_id"))}
		if v := strArg(a, "external_id"); v != "" {
			body["external_id"] = v
		}
		if v := strArg(a, "entity_type"); v != "" {
			body["entity_type"] = v
		}
		return rawResult(c.PostRaw(ctx, "/v1/anchors", body))
	}
}

func listAnchorsTool() Tool {
	return Tool{Name: "list_anchors", Description: "List the subjects (anchors) this tenant has memories about.",
		InputSchema: obj(map[string]interface{}{
			"entity_type": strF("Optional filter by type."),
			"limit":       intF("Max anchors. Defaults to 50."),
		})}
}
func listAnchorsHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		p := url.Values{}
		p.Set("limit", strconv.Itoa(intArg(a, "limit", 50)))
		if v := strArg(a, "entity_type"); v != "" {
			p.Set("entity_type", v)
		}
		return rawResult(c.GetRaw(ctx, "/v1/anchors?"+p.Encode()))
	}
}

func forgetSubjectTool() Tool {
	return Tool{Name: "forget_subject", Description: "GDPR / right-to-be-forgotten: permanently erase ALL memories about one subject (anchor) by its UUID. This is irreversible. A record that the erasure occurred is retained in the tamper-evident audit trail.",
		InputSchema: obj(map[string]interface{}{"anchor_id": strF("UUID of the anchor (subject) to erase.")}, "anchor_id")}
}
func forgetSubjectHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		id, _ := a["anchor_id"].(string)
		if id == "" {
			return ErrorResult("anchor_id is required"), nil
		}
		return rawResult(c.DeleteRaw(ctx, "/v1/anchors/"+url.PathEscape(id)+"?purge=true"))
	}
}

// ─── sessions ────────────────────────────────────────────────────────────────

func createSessionTool() Tool {
	return Tool{Name: "create_session", Description: "Start a conversation/session for short-term memory. Memories bound to a session decay after it ends but are promoted to the subject's durable profile if they recur. Returns the session UUID to pass to remember/recall.",
		InputSchema: obj(map[string]interface{}{
			"agent_id":    strF("Agent ID. Uses the default agent if omitted."),
			"anchor":      strF("Optional subject (your external id) this session is with."),
			"external_id": strF("Optional your-own id for the session."),
		})}
}
func createSessionHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		body := map[string]interface{}{"agent_id": c.ResolveAgent(strArg(a, "agent_id"))}
		if v := strArg(a, "anchor"); v != "" {
			body["anchor_external_id"] = v
		}
		if v := strArg(a, "external_id"); v != "" {
			body["external_id"] = v
		}
		return rawResult(c.PostRaw(ctx, "/v1/sessions", body))
	}
}

func endSessionTool() Tool {
	return Tool{Name: "end_session", Description: "End a session by its UUID, triggering promotion of recurring short-term memories into the subject's durable profile.",
		InputSchema: obj(map[string]interface{}{"session_id": strF("UUID of the session to end.")}, "session_id")}
}
func endSessionHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		id, _ := a["session_id"].(string)
		if id == "" {
			return ErrorResult("session_id is required"), nil
		}
		return rawResult(c.PostRaw(ctx, "/v1/sessions/"+url.PathEscape(id)+"/end", nil))
	}
}

// ─── agents & canon ──────────────────────────────────────────────────────────

func createAgentTool() Tool {
	return Tool{Name: "create_agent", Description: "Create a new agent under your tenant. Returns the agent UUID to use with the other tools.",
		InputSchema: obj(map[string]interface{}{
			"name":        strF("Human-readable agent name."),
			"external_id": strF("Optional your-own id for the agent."),
		}, "name")}
}
func createAgentHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		name, _ := a["name"].(string)
		if name == "" {
			return ErrorResult("name is required"), nil
		}
		body := map[string]interface{}{"name": name}
		if v := strArg(a, "external_id"); v != "" {
			body["external_id"] = v
		}
		return rawResult(c.PostRaw(ctx, "/v1/agents", body))
	}
}

func listCanonTool() Tool {
	return Tool{Name: "list_canon", Description: "List canon — tenant-wide authoritative knowledge shared across all of the tenant's agents (e.g. company policies, product facts).",
		InputSchema: obj(map[string]interface{}{})}
}
func listCanonHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		return rawResult(c.GetRaw(ctx, "/v1/canon"))
	}
}

// ─── working memory, schemas, knowledge graph ─────────────────────────────────

func activateContextTool() Tool {
	return Tool{Name: "activate_context", Description: "Load an agent's working memory for the current moment: given cues and an optional goal, spreading activation selects the most relevant memories/episodes/skills and returns an assembled context block ready to inject into a prompt.",
		InputSchema: obj(map[string]interface{}{
			"cues": map[string]interface{}{
				"type":        "array",
				"description": "Salient terms/phrases from the current turn to activate memory from.",
				"items":       map[string]interface{}{"type": "string"},
			},
			"goal":     strF("The agent's current goal, to bias activation. Optional."),
			"agent_id": strF("Agent ID. Uses the default agent if omitted."),
		}, "cues")}
}
func activateContextHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		rawCues, _ := a["cues"].([]interface{})
		cues := make([]string, 0, len(rawCues))
		for _, c := range rawCues {
			if s, ok := c.(string); ok {
				cues = append(cues, s)
			}
		}
		if len(cues) == 0 {
			return ErrorResult("cues is required (non-empty array of strings)"), nil
		}
		body := map[string]interface{}{"agent_id": c.ResolveAgent(strArg(a, "agent_id")), "cues": cues}
		if v := strArg(a, "goal"); v != "" {
			body["goal"] = v
		}
		return rawResult(c.PostRaw(ctx, "/v1/cognitive/activate", body))
	}
}

func listSchemasTool() Tool {
	return Tool{Name: "list_schemas", Description: "List an agent's schemas — higher-order mental models (user archetypes, situation templates, causal models) formed by clustering related memories.",
		InputSchema: obj(map[string]interface{}{"agent_id": strF("Agent ID. Uses the default agent if omitted.")})}
}
func listSchemasHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		return rawResult(c.GetRaw(ctx, "/v1/schemas?agent_id="+url.QueryEscape(c.ResolveAgent(strArg(a, "agent_id")))))
	}
}

func matchSchemasTool() Tool {
	return Tool{Name: "match_schemas", Description: "Find the mental models (schemas) that apply to the current query/context — e.g. which user archetype or situation template fits right now.",
		InputSchema: obj(map[string]interface{}{
			"query":    strF("The current situation/context to match schemas against."),
			"agent_id": strF("Agent ID. Uses the default agent if omitted."),
			"limit":    intF("Max schemas. Defaults to 5."),
		}, "query")}
}
func matchSchemasHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		q, _ := a["query"].(string)
		if q == "" {
			return ErrorResult("query is required"), nil
		}
		body := map[string]interface{}{"agent_id": c.ResolveAgent(strArg(a, "agent_id")), "query": q, "limit": intArg(a, "limit", 5)}
		return rawResult(c.PostRaw(ctx, "/v1/schemas/match", body))
	}
}
 
func listEntitiesTool() Tool {
	return Tool{Name: "list_entities", Description: "List the entities (people, things, concepts) extracted into an agent's knowledge graph, with how many memories mention each.",
		InputSchema: obj(map[string]interface{}{"agent_id": strF("Agent ID. Uses the default agent if omitted.")})}
}
func listEntitiesHandler(c *Client) ToolHandler {
	return func(ctx context.Context, a map[string]interface{}) (*CallToolResult, error) {
		return rawResult(c.GetRaw(ctx, "/v1/graph/entities?agent_id="+url.QueryEscape(c.ResolveAgent(strArg(a, "agent_id")))))
	}
}

// strArg returns a string argument or "".
func strArg(a map[string]interface{}, key string) string {
	s, _ := a[key].(string)
	return strings.TrimSpace(s)
}
