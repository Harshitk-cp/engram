package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Harshitk-cp/engram/mcp"
	"github.com/Harshitk-cp/engram/mcp/transports"
)

// ─── Mock Engram API ────────────────────────────────────────────────────────────

// engramMock is a lightweight fake of the Engram HTTP API.
// Each test configures it by setting the relevant response fields.
type engramMock struct {
	t *testing.T

	// Per-endpoint responses (set before the test call)
	memoryCreateResp   interface{}
	memoryCreateStatus int
	recallResp         interface{}
	recallStatus       int
	deleteStatus       int
	hotMemoriesResp    interface{}
	hotMemoriesStatus  int
	agentsResp         interface{}
	agentsStatus       int
	healthResp         interface{}
	healthStatus       int
	auditResp          interface{}

	// Request capture
	lastMethod string
	lastPath   string
	lastBody   map[string]interface{}
}

func newMock(t *testing.T) *engramMock {
	t.Helper()
	return &engramMock{
		t:                  t,
		memoryCreateStatus: 201,
		recallStatus:       200,
		deleteStatus:       204,
		hotMemoriesStatus:  200,
		agentsStatus:       200,
		healthStatus:       200,
	}
}

func (m *engramMock) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.lastMethod = r.Method
		m.lastPath = r.URL.Path

		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&m.lastBody)
		}

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/memories":
			write(w, m.memoryCreateStatus, m.memoryCreateResp)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/memories/recall":
			write(w, m.recallStatus, m.recallResp)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/memories/"):
			w.WriteHeader(m.deleteStatus)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/hot-memories"):
			write(w, m.hotMemoriesStatus, m.hotMemoriesResp)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/cognitive/health":
			write(w, m.healthStatus, m.healthResp)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/audit/verify":
			write(w, 200, m.auditResp)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/agents":
			write(w, m.agentsStatus, m.agentsResp)
		default:
			m.t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func write(w http.ResponseWriter, status int, body interface{}) {
	if body == nil {
		w.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// ─── Helpers ───────────────────────────────────────────────────────────────────

func newServer(t *testing.T, client *mcp.Client) *mcp.Server {
	t.Helper()
	s := mcp.NewServer("engram", "0.1.0")
	mcp.RegisterTools(s, client)
	mcp.RegisterResources(s, client)
	return s
}

func callTool(t *testing.T, s *mcp.Server, name string, args map[string]interface{}) *mcp.CallToolResult {
	t.Helper()
	argsJSON, _ := json.Marshal(args)
	paramsJSON, _ := json.Marshal(map[string]interface{}{"name": name, "arguments": args})
	_ = argsJSON

	req := &mcp.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params:  json.RawMessage(paramsJSON),
	}
	resp := s.Handle(context.Background(), req)
	if resp == nil {
		t.Fatal("Handle returned nil for a request with ID")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %d %s", resp.Error.Code, resp.Error.Message)
	}

	data, _ := json.Marshal(resp.Result)
	var result mcp.CallToolResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal tool result: %v", err)
	}
	return &result
}

func assertText(t *testing.T, result *mcp.CallToolResult, contains string) {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	text := result.Content[0].Text
	if !strings.Contains(text, contains) {
		t.Errorf("expected text to contain %q, got:\n%s", contains, text)
	}
}

func assertError(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	if !result.IsError {
		t.Errorf("expected IsError=true, text: %s", result.Content[0].Text)
	}
}

func assertNoError(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	if result.IsError {
		t.Errorf("expected IsError=false, text: %s", result.Content[0].Text)
	}
}

// ─── Tool tests ────────────────────────────────────────────────────────────────

func TestRemember_Success(t *testing.T) {
	mock := newMock(t)
	mock.memoryCreateResp = map[string]interface{}{
		"id":          "mem-abc-123",
		"content":     "User prefers dark mode",
		"type":        "preference",
		"confidence":  0.9,
		"tier":        "hot",
		"tier_reason": "high confidence — frequently accessed",
		"reinforced":  false,
	}
	srv := mock.server()
	defer srv.Close()

	client := mcp.NewClient(srv.URL, "mk_test", "agent-1")
	s := newServer(t, client)

	result := callTool(t, s, "remember", map[string]interface{}{
		"content": "User prefers dark mode",
		"type":    "preference",
	})

	assertNoError(t, result)
	assertText(t, result, "mem-abc-123")
	assertText(t, result, "hot")
	assertText(t, result, "0.90")

	if mock.lastMethod != "POST" || mock.lastPath != "/v1/memories" {
		t.Errorf("unexpected API call: %s %s", mock.lastMethod, mock.lastPath)
	}
	if mock.lastBody["content"] != "User prefers dark mode" {
		t.Errorf("wrong content in request body: %v", mock.lastBody)
	}
}

func TestRemember_Reinforced(t *testing.T) {
	mock := newMock(t)
	mock.memoryCreateResp = map[string]interface{}{
		"id": "mem-xyz", "content": "test", "tier": "hot",
		"confidence": 0.95, "reinforced": true,
	}
	srv := mock.server()
	defer srv.Close()

	result := callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "a")), "remember",
		map[string]interface{}{"content": "test"})

	assertNoError(t, result)
	assertText(t, result, "reinforced")
}

func TestRemember_MissingContent(t *testing.T) {
	mock := newMock(t)
	srv := mock.server()
	defer srv.Close()

	result := callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "a")), "remember",
		map[string]interface{}{})

	assertError(t, result)
	assertText(t, result, "content is required")
}

func TestRemember_DefaultSource(t *testing.T) {
	mock := newMock(t)
	mock.memoryCreateResp = map[string]interface{}{"id": "x", "content": "y", "tier": "hot", "confidence": 0.9}
	srv := mock.server()
	defer srv.Close()

	callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "a")), "remember",
		map[string]interface{}{"content": "hello"})

	if mock.lastBody["source"] != "agent" {
		t.Errorf("expected default source=agent, got %v", mock.lastBody["source"])
	}
}

func TestRemember_CustomAgentID(t *testing.T) {
	mock := newMock(t)
	mock.memoryCreateResp = map[string]interface{}{"id": "x", "content": "y", "tier": "hot", "confidence": 0.9}
	srv := mock.server()
	defer srv.Close()

	callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "default-agent")), "remember",
		map[string]interface{}{"content": "test", "agent_id": "custom-agent-99"})

	if mock.lastBody["agent_id"] != "custom-agent-99" {
		t.Errorf("expected custom agent_id in request, got %v", mock.lastBody["agent_id"])
	}
}

func TestRemember_APIError(t *testing.T) {
	mock := newMock(t)
	mock.memoryCreateStatus = 500
	mock.memoryCreateResp = map[string]interface{}{"error": "internal server error"}
	srv := mock.server()
	defer srv.Close()

	result := callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "a")), "remember",
		map[string]interface{}{"content": "test"})

	assertError(t, result)
}

func TestRecall_Success(t *testing.T) {
	mock := newMock(t)
	mock.recallResp = map[string]interface{}{
		"memories": []interface{}{
			map[string]interface{}{
				"id": "m1", "content": "User is a Go developer",
				"type": "fact", "confidence": 0.92, "score": 0.87,
			},
			map[string]interface{}{
				"id": "m2", "content": "User prefers TDD",
				"type": "preference", "confidence": 0.75, "score": 0.71,
			},
		},
	}
	srv := mock.server()
	defer srv.Close()

	result := callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "a")), "recall",
		map[string]interface{}{"query": "programming style"})

	assertNoError(t, result)
	assertText(t, result, "Found 2 memories")
	assertText(t, result, "User is a Go developer")
	assertText(t, result, "User prefers TDD")
	assertText(t, result, "0.92") // confidence formatted to 2dp
	assertText(t, result, "0.87") // score formatted to 2dp (0.87 rounds from 0.870)
}

func TestRecall_Empty(t *testing.T) {
	mock := newMock(t)
	mock.recallResp = map[string]interface{}{"memories": []interface{}{}}
	srv := mock.server()
	defer srv.Close()

	result := callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "a")), "recall",
		map[string]interface{}{"query": "nothing here"})

	assertNoError(t, result)
	assertText(t, result, "No memories found")
}

func TestRecall_MissingQuery(t *testing.T) {
	mock := newMock(t)
	srv := mock.server()
	defer srv.Close()

	result := callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "a")), "recall",
		map[string]interface{}{})

	assertError(t, result)
	assertText(t, result, "query is required")
}

func TestRecall_DefaultsToNoGraph(t *testing.T) {
	mock := newMock(t)
	mock.recallResp = map[string]interface{}{"memories": []interface{}{}}
	srv := mock.server()
	defer srv.Close()

	callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "a")), "recall",
		map[string]interface{}{"query": "test"})

	if strings.Contains(mock.lastPath, "graph_weight") {
		t.Error("recall should not send graph_weight=0")
	}
}

func TestRecallGraph_UsesGraphWeight(t *testing.T) {
	var capturedQuery string
	var capturedGraphWeight string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query().Get("query")
		capturedGraphWeight = r.URL.Query().Get("graph_weight")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"memories": []interface{}{}})
	}))
	defer srv.Close()

	callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "a")), "recall_graph",
		map[string]interface{}{"query": "semantic links"})

	if capturedQuery != "semantic links" {
		t.Errorf("wrong query sent: %q", capturedQuery)
	}
	if capturedGraphWeight == "" || capturedGraphWeight == "0" {
		t.Errorf("recall_graph should send graph_weight>0, got %q", capturedGraphWeight)
	}
}

func TestRecallGraph_CustomWeight(t *testing.T) {
	var capturedWeight string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedWeight = r.URL.Query().Get("graph_weight")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"memories": []interface{}{}})
	}))
	defer srv.Close()

	callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "a")), "recall_graph",
		map[string]interface{}{"query": "test", "graph_weight": 0.7})

	if capturedWeight != "0.70" {
		t.Errorf("expected graph_weight=0.70, got %q", capturedWeight)
	}
}

func TestForget_Success(t *testing.T) {
	mock := newMock(t)
	mock.deleteStatus = 204
	srv := mock.server()
	defer srv.Close()

	result := callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "a")), "forget",
		map[string]interface{}{"memory_id": "mem-to-delete"})

	assertNoError(t, result)
	assertText(t, result, "mem-to-delete")
	assertText(t, result, "deleted")

	if mock.lastPath != "/v1/memories/mem-to-delete" {
		t.Errorf("wrong delete path: %s", mock.lastPath)
	}
}

func TestForget_MissingID(t *testing.T) {
	mock := newMock(t)
	srv := mock.server()
	defer srv.Close()

	result := callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "a")), "forget",
		map[string]interface{}{})

	assertError(t, result)
	assertText(t, result, "memory_id is required")
}

func TestForget_APIError(t *testing.T) {
	mock := newMock(t)
	mock.deleteStatus = 404
	srv := mock.server()
	defer srv.Close()

	result := callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "a")), "forget",
		map[string]interface{}{"memory_id": "gone"})

	assertError(t, result)
}

func TestGetHotContext_WithMemories(t *testing.T) {
	mock := newMock(t)
	mock.hotMemoriesResp = map[string]interface{}{
		"memories": []interface{}{
			map[string]interface{}{"id": "h1", "content": "User is senior backend engineer", "type": "fact", "confidence": 0.95},
			map[string]interface{}{"id": "h2", "content": "User prefers concise responses", "type": "preference", "confidence": 0.88},
		},
	}
	srv := mock.server()
	defer srv.Close()

	result := callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "agent-99")), "get_hot_context",
		map[string]interface{}{})

	assertNoError(t, result)
	assertText(t, result, "Known context")
	assertText(t, result, "User is senior backend engineer")
	assertText(t, result, "User prefers concise responses")
	assertText(t, result, "0.95")

	if !strings.Contains(mock.lastPath, "agent-99") {
		t.Errorf("expected default agent in path, got %s", mock.lastPath)
	}
}

func TestGetHotContext_Empty(t *testing.T) {
	mock := newMock(t)
	mock.hotMemoriesResp = map[string]interface{}{"memories": []interface{}{}}
	srv := mock.server()
	defer srv.Close()

	result := callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "a")), "get_hot_context",
		map[string]interface{}{})

	assertNoError(t, result)
	assertText(t, result, "No hot-tier memories")
}

func TestGetHotContext_CustomAgentAndLimit(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"memories": []interface{}{}})
	}))
	defer srv.Close()

	callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "default")), "get_hot_context",
		map[string]interface{}{"agent_id": "custom-agent", "limit": 5})

	if !strings.Contains(capturedPath, "custom-agent") {
		t.Errorf("expected custom-agent in path, got %s", capturedPath)
	}
	if !strings.Contains(capturedPath, "limit=5") {
		t.Errorf("expected limit=5 in path, got %s", capturedPath)
	}
}

func TestListAgents_Success(t *testing.T) {
	mock := newMock(t)
	mock.agentsResp = map[string]interface{}{
		"agents": []interface{}{
			map[string]interface{}{"id": "a1", "external_id": "ext-1", "name": "Alpha Agent"},
			map[string]interface{}{"id": "a2", "external_id": "ext-2", "name": "Beta Agent"},
		},
	}
	srv := mock.server()
	defer srv.Close()

	result := callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "a")), "list_agents",
		map[string]interface{}{})

	assertNoError(t, result)
	assertText(t, result, "Alpha Agent")
	assertText(t, result, "Beta Agent")
	assertText(t, result, "ext-1")
}

func TestListAgents_Empty(t *testing.T) {
	mock := newMock(t)
	mock.agentsResp = map[string]interface{}{"agents": []interface{}{}}
	srv := mock.server()
	defer srv.Close()

	result := callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "a")), "list_agents",
		map[string]interface{}{})

	assertNoError(t, result)
	assertText(t, result, "No agents found")
}

func TestListAgents_APIError(t *testing.T) {
	mock := newMock(t)
	mock.agentsStatus = 401
	mock.agentsResp = map[string]interface{}{"error": "unauthorized"}
	srv := mock.server()
	defer srv.Close()

	result := callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "a")), "list_agents",
		map[string]interface{}{})

	assertError(t, result)
}

// ─── Unknown tool ───────────────────────────────────────────────────────────────

func TestUnknownTool(t *testing.T) {
	s := mcp.NewServer("engram", "0.1.0")
	paramsJSON, _ := json.Marshal(map[string]interface{}{"name": "nonexistent", "arguments": map[string]interface{}{}})
	req := &mcp.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params:  json.RawMessage(paramsJSON),
	}
	resp := s.Handle(context.Background(), req)
	if resp.Error == nil {
		t.Fatal("expected RPC error for unknown tool")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected code -32601 (method not found), got %d", resp.Error.Code)
	}
}

// ─── Resource tests ─────────────────────────────────────────────────────────────

func TestResourcesList(t *testing.T) {
	mock := newMock(t)
	srv := mock.server()
	defer srv.Close()

	s := newServer(t, mcp.NewClient(srv.URL, "k", "a"))
	req := &mcp.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "resources/list",
	}
	resp := s.Handle(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	data, _ := json.Marshal(resp.Result)
	var result struct {
		Resources []struct {
			URI  string `json:"uri"`
			Name string `json:"name"`
		} `json:"resources"`
	}
	_ = json.Unmarshal(data, &result)

	if len(result.Resources) != 4 {
		t.Errorf("expected 4 resources, got %d", len(result.Resources))
	}

	uris := map[string]bool{}
	for _, r := range result.Resources {
		uris[r.URI] = true
	}
	for _, want := range []string{
		"engram://agents/{agent_id}/memories",
		"engram://agents/{agent_id}/health",
		"engram://agents/{agent_id}/calibration",
		"engram://audit/integrity",
	} {
		if !uris[want] {
			t.Errorf("missing resource %s", want)
		}
	}
}

func TestResourcesRead_Memories(t *testing.T) {
	mock := newMock(t)
	mock.hotMemoriesResp = map[string]interface{}{
		"memories": []interface{}{
			map[string]interface{}{"id": "m1", "content": "User is a Gopher", "confidence": 0.9},
		},
	}
	srv := mock.server()
	defer srv.Close()

	s := newServer(t, mcp.NewClient(srv.URL, "k", "a"))
	paramsJSON, _ := json.Marshal(map[string]interface{}{
		"uri": "engram://agents/agent-42/memories",
	})
	req := &mcp.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "resources/read",
		Params:  json.RawMessage(paramsJSON),
	}
	resp := s.Handle(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	data, _ := json.Marshal(resp.Result)
	text := string(data)
	if !strings.Contains(text, "User is a Gopher") {
		t.Errorf("expected memory content in response, got: %s", text)
	}
}

func TestResourcesRead_Health(t *testing.T) {
	mock := newMock(t)
	mock.healthResp = map[string]interface{}{
		"semantic_count":      12,
		"average_confidence":  0.82,
		"contradiction_count": 1,
	}
	srv := mock.server()
	defer srv.Close()

	s := newServer(t, mcp.NewClient(srv.URL, "k", "a"))
	paramsJSON, _ := json.Marshal(map[string]interface{}{
		"uri": "engram://agents/agent-77/health",
	})
	req := &mcp.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "resources/read",
		Params:  json.RawMessage(paramsJSON),
	}
	resp := s.Handle(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	data, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(data), "average_confidence") {
		t.Errorf("expected knowledge-health fields in response: %s", data)
	}
	if mock.lastPath != "/v1/cognitive/health" {
		t.Errorf("expected health resource to call /v1/cognitive/health, got %s", mock.lastPath)
	}
}

func TestResourcesRead_UnknownURI(t *testing.T) {
	mock := newMock(t)
	srv := mock.server()
	defer srv.Close()

	s := newServer(t, mcp.NewClient(srv.URL, "k", "a"))
	paramsJSON, _ := json.Marshal(map[string]interface{}{"uri": "engram://unknown/resource"})
	req := &mcp.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "resources/read",
		Params:  json.RawMessage(paramsJSON),
	}
	resp := s.Handle(context.Background(), req)
	if resp.Error == nil {
		t.Fatal("expected error for unknown resource URI")
	}
}

// ─── MCP protocol tests ─────────────────────────────────────────────────────────

func TestInitialize(t *testing.T) {
	s := mcp.NewServer("engram", "0.1.0")
	req := &mcp.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}`),
	}
	resp := s.Handle(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("initialize failed: %v", resp.Error)
	}

	data, _ := json.Marshal(resp.Result)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)

	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("wrong protocolVersion: %v", result["protocolVersion"])
	}
	info := result["serverInfo"].(map[string]interface{})
	if info["name"] != "engram" {
		t.Errorf("wrong server name: %v", info["name"])
	}
	// Instructions must ship in initialize — this is what makes hosts auto-adopt
	// Engram as memory (recall-at-start / remember-on-fact) without prompt wiring.
	instr, ok := result["instructions"].(string)
	if !ok || instr == "" {
		t.Fatalf("initialize must include non-empty instructions, got %v", result["instructions"])
	}
	if !strings.Contains(instr, "recall") || !strings.Contains(instr, "remember") {
		t.Errorf("instructions should direct the agent to recall and remember, got: %q", instr)
	}
}

func TestInitialize_InstructionsDisabled(t *testing.T) {
	t.Setenv("ENGRAM_MCP_INSTRUCTIONS", "")
	s := mcp.NewServer("engram", "0.1.0")
	req := &mcp.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}`),
	}
	resp := s.Handle(context.Background(), req)
	data, _ := json.Marshal(resp.Result)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	if _, present := result["instructions"]; present {
		t.Errorf("instructions should be omitted when ENGRAM_MCP_INSTRUCTIONS is empty, got %v", result["instructions"])
	}
}

func TestPing(t *testing.T) {
	s := mcp.NewServer("engram", "0.1.0")
	req := &mcp.Request{JSONRPC: "2.0", ID: json.RawMessage(`99`), Method: "ping"}
	resp := s.Handle(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("ping failed: %v", resp.Error)
	}
}

func TestNotification_NoResponse(t *testing.T) {
	s := mcp.NewServer("engram", "0.1.0")
	// notifications have no ID
	req := &mcp.Request{JSONRPC: "2.0", Method: "notifications/initialized"}
	resp := s.Handle(context.Background(), req)
	if resp != nil {
		t.Errorf("expected nil response for notification, got: %+v", resp)
	}
}

func TestUnknownMethod(t *testing.T) {
	s := mcp.NewServer("engram", "0.1.0")
	req := &mcp.Request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "bogus/method"}
	resp := s.Handle(context.Background(), req)
	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected -32601, got %d", resp.Error.Code)
	}
}

func TestToolsList(t *testing.T) {
	s := mcp.NewServer("engram", "0.1.0")
	client := mcp.NewClient("http://localhost", "k", "a")
	mcp.RegisterTools(s, client)

	req := &mcp.Request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/list"}
	resp := s.Handle(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("tools/list failed: %v", resp.Error)
	}

	data, _ := json.Marshal(resp.Result)
	var result struct {
		Tools []struct {
			Name        string      `json:"name"`
			Description string      `json:"description"`
			InputSchema interface{} `json:"inputSchema"`
		} `json:"tools"`
	}
	_ = json.Unmarshal(data, &result)

	wantTools := []string{"remember", "recall", "recall_graph", "forget", "get_hot_context", "list_agents"}
	got := map[string]bool{}
	for _, tool := range result.Tools {
		got[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("tool %q has empty description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %q has nil inputSchema", tool.Name)
		}
	}
	for _, name := range wantTools {
		if !got[name] {
			t.Errorf("missing tool: %q", name)
		}
	}
}

// ─── Stdio transport tests ──────────────────────────────────────────────────────

func stdioRoundtrip(t *testing.T, s *mcp.Server, requests ...interface{}) []map[string]interface{} {
	t.Helper()

	var input bytes.Buffer
	for _, req := range requests {
		line, _ := json.Marshal(req)
		input.Write(line)
		input.WriteByte('\n')
	}

	var output bytes.Buffer
	err := transports.StdioRW(context.Background(), s, &input, &output)
	if err != nil {
		t.Fatalf("StdioRW: %v", err)
	}

	var results []map[string]interface{}
	dec := json.NewDecoder(&output)
	for {
		var m map[string]interface{}
		if err := dec.Decode(&m); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("decode output: %v", err)
		}
		results = append(results, m)
	}
	return results
}

func TestStdio_Initialize(t *testing.T) {
	s := mcp.NewServer("engram", "0.1.0")

	results := stdioRoundtrip(t, s, map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo":      map[string]interface{}{"name": "test", "version": "0"},
		},
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 response, got %d", len(results))
	}
	result := results[0]["result"].(map[string]interface{})
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("wrong protocolVersion: %v", result["protocolVersion"])
	}
}

func TestStdio_Notification_NoResponse(t *testing.T) {
	s := mcp.NewServer("engram", "0.1.0")

	// Notification (no id) followed by a ping (has id) — only ping gets a response
	results := stdioRoundtrip(t, s,
		map[string]interface{}{"jsonrpc": "2.0", "method": "notifications/initialized"},
		map[string]interface{}{"jsonrpc": "2.0", "id": 5, "method": "ping"},
	)

	if len(results) != 1 {
		t.Errorf("expected 1 response (only to ping), got %d: %v", len(results), results)
	}
	if results[0]["id"].(float64) != 5 {
		t.Errorf("response ID mismatch: %v", results[0]["id"])
	}
}

func TestStdio_ParseError(t *testing.T) {
	s := mcp.NewServer("engram", "0.1.0")

	var input bytes.Buffer
	input.WriteString("this is not valid json\n")
	input.WriteString(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")

	var output bytes.Buffer
	_ = transports.StdioRW(context.Background(), s, &input, &output)

	var results []map[string]interface{}
	dec := json.NewDecoder(&output)
	for {
		var m map[string]interface{}
		if err := dec.Decode(&m); err == io.EOF {
			break
		} else if err == nil {
			results = append(results, m)
		}
	}

	if len(results) < 2 {
		t.Fatalf("expected parse error response + ping response, got %d", len(results))
	}
	// First response is the parse error
	if results[0]["error"] == nil {
		t.Error("expected parse error in first response")
	}
	// Second response is the ping
	if results[1]["result"] == nil {
		t.Error("expected result in second response (ping)")
	}
}

func TestStdio_MultipleRequests(t *testing.T) {
	mock := newMock(t)
	mock.memoryCreateResp = map[string]interface{}{"id": "m1", "content": "x", "tier": "hot", "confidence": 0.9}
	mock.recallResp = map[string]interface{}{"memories": []interface{}{}}
	srv := mock.server()
	defer srv.Close()

	s := newServer(t, mcp.NewClient(srv.URL, "k", "a"))

	results := stdioRoundtrip(t, s,
		map[string]interface{}{
			"jsonrpc": "2.0", "id": 1, "method": "initialize",
			"params": map[string]interface{}{"protocolVersion": "2024-11-05", "capabilities": map[string]interface{}{}},
		},
		map[string]interface{}{"jsonrpc": "2.0", "method": "notifications/initialized"},
		map[string]interface{}{"jsonrpc": "2.0", "id": 2, "method": "tools/list"},
		map[string]interface{}{
			"jsonrpc": "2.0", "id": 3, "method": "tools/call",
			"params": map[string]interface{}{
				"name":      "recall",
				"arguments": map[string]interface{}{"query": "test query"},
			},
		},
	)

	// initialize + tools/list + tools/call = 3 responses (notification has no response)
	if len(results) != 3 {
		t.Errorf("expected 3 responses, got %d: %v", len(results), results)
	}
	ids := []float64{}
	for _, r := range results {
		if id, ok := r["id"].(float64); ok {
			ids = append(ids, id)
		}
	}
	if len(ids) != 3 || ids[0] != 1 || ids[1] != 2 || ids[2] != 3 {
		t.Errorf("wrong response IDs: %v", ids)
	}
}

func TestStdio_EmptyLines(t *testing.T) {
	s := mcp.NewServer("engram", "0.1.0")

	var input bytes.Buffer
	input.WriteString("\n")
	input.WriteString("   \n")
	input.WriteString(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")

	var output bytes.Buffer
	_ = transports.StdioRW(context.Background(), s, &input, &output)

	var results []map[string]interface{}
	dec := json.NewDecoder(&output)
	for {
		var m map[string]interface{}
		if err := dec.Decode(&m); err == io.EOF {
			break
		} else if err == nil {
			results = append(results, m)
		}
	}

	// Empty lines are skipped; only the ping gets a response
	// (empty lines don't parse as JSON, they produce parse errors)
	pings := 0
	for _, r := range results {
		if r["result"] != nil {
			pings++
		}
	}
	if pings != 1 {
		t.Errorf("expected exactly 1 ping response, results: %v", results)
	}
}

// ─── HTTP transport tests ───────────────────────────────────────────────────────

func TestHTTPTransport_MCP(t *testing.T) {
	mock := newMock(t)
	mock.agentsResp = map[string]interface{}{"agents": []interface{}{
		map[string]interface{}{"id": "a1", "name": "Agent One"},
	}}
	apiSrv := mock.server()
	defer apiSrv.Close()

	s := newServer(t, mcp.NewClient(apiSrv.URL, "k", "a"))
	httpSrv := transports.NewHTTPServer(s)
	ts := httptest.NewServer(httpSrv.Handler())
	defer ts.Close()

	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]interface{}{
			"name":      "list_agents",
			"arguments": map[string]interface{}{},
		},
	})

	resp, err := http.Post(ts.URL+"/mcp", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)

	if result["error"] != nil {
		t.Errorf("unexpected error: %v", result["error"])
	}
	data, _ := json.Marshal(result["result"])
	if !strings.Contains(string(data), "Agent One") {
		t.Errorf("expected Agent One in response: %s", data)
	}
}

func TestHTTPTransport_Health(t *testing.T) {
	s := mcp.NewServer("engram", "0.1.0")
	httpSrv := transports.NewHTTPServer(s)
	ts := httptest.NewServer(httpSrv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHTTPTransport_InvalidJSON(t *testing.T) {
	s := mcp.NewServer("engram", "0.1.0")
	httpSrv := transports.NewHTTPServer(s)
	ts := httptest.NewServer(httpSrv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/mcp", "application/json",
		strings.NewReader("not json"))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", resp.StatusCode)
	}
}

func TestHTTPTransport_WrongMethod(t *testing.T) {
	s := mcp.NewServer("engram", "0.1.0")
	httpSrv := transports.NewHTTPServer(s)
	ts := httptest.NewServer(httpSrv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/mcp")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}

func TestHTTPTransport_Notification(t *testing.T) {
	s := mcp.NewServer("engram", "0.1.0")
	httpSrv := transports.NewHTTPServer(s)
	ts := httptest.NewServer(httpSrv.Handler())
	defer ts.Close()

	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		// no id — notification
	})

	resp, err := http.Post(ts.URL+"/mcp", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204 for notification, got %d", resp.StatusCode)
	}
}

// ─── Client defaults ────────────────────────────────────────────────────────────

func TestClient_DefaultAgentFallback(t *testing.T) {
	var capturedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "m1", "content": "x", "tier": "hot", "confidence": 0.9,
		})
	}))
	defer srv.Close()

	// Default agent is "default-agent"; no agent_id in tool args
	client := mcp.NewClient(srv.URL, "mk_test", "default-agent")
	s := newServer(t, client)

	callTool(t, s, "remember", map[string]interface{}{"content": "test memory"})

	if capturedBody["agent_id"] != "default-agent" {
		t.Errorf("expected default agent_id=default-agent in request, got %v", capturedBody["agent_id"])
	}
}

func TestClient_AuthorizationHeader(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"agents": []interface{}{}})
	}))
	defer srv.Close()

	client := mcp.NewClient(srv.URL, "mk_secretkey123", "a")
	s := newServer(t, client)
	callTool(t, s, "list_agents", map[string]interface{}{})

	if capturedAuth != "Bearer mk_secretkey123" {
		t.Errorf("expected Authorization: Bearer mk_secretkey123, got %q", capturedAuth)
	}
}

// ─── Edge cases ─────────────────────────────────────────────────────────────────

func TestToolsCall_InvalidParams(t *testing.T) {
	s := mcp.NewServer("engram", "0.1.0")
	req := &mcp.Request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params:  json.RawMessage(`not valid json`),
	}
	resp := s.Handle(context.Background(), req)
	if resp.Error == nil {
		t.Fatal("expected error for invalid params")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("expected -32602 (invalid params), got %d", resp.Error.Code)
	}
}

func TestRecall_TopK(t *testing.T) {
	var capturedTopK string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTopK = r.URL.Query().Get("top_k")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"memories": []interface{}{}})
	}))
	defer srv.Close()

	callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "a")), "recall",
		map[string]interface{}{"query": "test", "top_k": 25})

	if capturedTopK != "25" {
		t.Errorf("expected top_k=25, got %q", capturedTopK)
	}
}

func TestFormatMemoriesIDs(t *testing.T) {
	mock := newMock(t)
	mock.recallResp = map[string]interface{}{
		"memories": []interface{}{
			map[string]interface{}{
				"id":      "550e8400-e29b-41d4-a716-446655440000",
				"content": "Important fact",
				"type":    "fact", "confidence": 0.8, "score": 0.75,
			},
		},
	}
	srv := mock.server()
	defer srv.Close()

	result := callTool(t, newServer(t, mcp.NewClient(srv.URL, "k", "a")), "recall",
		map[string]interface{}{"query": "fact"})

	// The full UUID should be present so users can reference it in forget()
	assertText(t, result, "550e8400-e29b-41d4-a716-446655440000")
}

// Fuzz-style: ensure the server doesn't panic on any method string
func TestServer_NoPanicOnAnyMethod(t *testing.T) {
	s := mcp.NewServer("engram", "0.1.0")
	methods := []string{
		"", "tools/", "/call", "TOOLS/LIST", "resources/Templates/list",
		strings.Repeat("x", 1000),
		"tools/call", // valid but no params
	}
	for _, m := range methods {
		t.Run(fmt.Sprintf("method=%q", m), func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("server panicked on method %q: %v", m, r)
				}
			}()
			req := &mcp.Request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: m}
			s.Handle(context.Background(), req)
		})
	}
}

func TestToolsList_RichSurface(t *testing.T) {
	mock := newMock(t)
	srv := mock.server()
	defer srv.Close()
	s := newServer(t, mcp.NewClient(srv.URL, "k", "a"))

	req := &mcp.Request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/list"}
	resp := s.Handle(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("tools/list error: %v", resp.Error)
	}
	data, _ := json.Marshal(resp.Result)
	var out struct {
		Tools []struct {
			Name        string      `json:"name"`
			Description string      `json:"description"`
			InputSchema interface{} `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}

	// Every advertised capability must be present.
	want := []string{
		"remember", "recall", "recall_graph", "forget", "get_hot_context", "list_agents",
		"get_memory", "list_memories", "explain_memory", "restore_memory", "record_feedback",
		"ingest_conversation", "recall_episodes", "record_episode", "match_procedures",
		"knowledge_health", "get_calibration", "list_contradictions", "verify_audit",
		"assess_confidence", "detect_uncertainty", "reflect", "consolidate",
		"create_anchor", "list_anchors", "forget_subject", "create_session", "end_session",
		"activate_context", "list_schemas", "match_schemas", "list_entities",
		"create_agent", "list_canon",
	}
	have := map[string]bool{}
	for _, tl := range out.Tools {
		have[tl.Name] = true
		if tl.Description == "" {
			t.Errorf("tool %s has no description", tl.Name)
		}
		if tl.InputSchema == nil {
			t.Errorf("tool %s has no input schema", tl.Name)
		}
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("expected tool %q to be registered", w)
		}
	}
	if len(out.Tools) < len(want) {
		t.Errorf("expected >= %d tools, got %d", len(want), len(out.Tools))
	}
}

func TestVerifyAuditTool(t *testing.T) {
	mock := newMock(t)
	mock.auditResp = map[string]interface{}{"valid": true, "checked": 42, "head_seq": 42}
	srv := mock.server()
	defer srv.Close()
	s := newServer(t, mcp.NewClient(srv.URL, "k", "a"))

	params, _ := json.Marshal(map[string]interface{}{"name": "verify_audit", "arguments": map[string]interface{}{}})
	req := &mcp.Request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call", Params: params}
	resp := s.Handle(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("verify_audit error: %v", resp.Error)
	}
	data, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(data), "\\\"valid\\\": true") && !strings.Contains(string(data), "valid") {
		t.Errorf("expected audit verification result, got: %s", data)
	}
	if mock.lastPath != "/v1/audit/verify" {
		t.Errorf("expected /v1/audit/verify, got %s", mock.lastPath)
	}
}
