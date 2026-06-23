package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Harshitk-cp/engram/mcp"
)

// post builds an MCP JSON-RPC POST with the given Authorization header.
func mcpPost(method, auth string) *http.Request {
	body, _ := json.Marshal(mcp.Request{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: method})
	r := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	if auth != "" {
		r.Header.Set("Authorization", auth)
	}
	return r
}

// initialize and tools/list don't touch the REST API, so they exercise the
// handler wiring (per-request server build + tool registration) without a DB.
func TestMCPHandler_InitializeAndToolsList(t *testing.T) {
	h := NewMCPHandler(&http.Client{})

	w := httptest.NewRecorder()
	h.Handle(w, mcpPost("initialize", "Bearer mk_test"))
	if w.Code != http.StatusOK {
		t.Fatalf("initialize status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var initResp mcp.Response
	if err := json.Unmarshal(w.Body.Bytes(), &initResp); err != nil {
		t.Fatalf("decode initialize: %v", err)
	}
	if initResp.Error != nil || initResp.Result == nil {
		t.Fatalf("initialize: err=%+v result=%v", initResp.Error, initResp.Result)
	}

	w = httptest.NewRecorder()
	h.Handle(w, mcpPost("tools/list", "Bearer mk_test"))
	if w.Code != http.StatusOK {
		t.Fatalf("tools/list status = %d, want 200", w.Code)
	}
	var listResp struct {
		Result struct {
			Tools []json.RawMessage `json:"tools"`
		} `json:"result"`
		Error *mcp.RPCError `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode tools/list: %v", err)
	}
	if listResp.Error != nil {
		t.Fatalf("tools/list rpc error: %+v", listResp.Error)
	}
	if len(listResp.Result.Tools) == 0 {
		t.Fatalf("expected registered MCP tools, got none")
	}
}

func TestMCPHandler_MissingKeyIs401(t *testing.T) {
	h := NewMCPHandler(&http.Client{})
	w := httptest.NewRecorder()
	h.Handle(w, mcpPost("initialize", "")) // no Authorization header
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}
