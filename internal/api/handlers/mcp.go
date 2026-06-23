package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Harshitk-cp/engram/mcp"
)

// MCPHandler serves the MCP Streamable HTTP transport in-process, so any MCP
// client can connect to <server>/mcp with just an Engram API key — no local
// engram-mcp binary, no env vars:
//
//	claude mcp add --transport http engram https://console.hakuya.ai/mcp \
//	  --header "Authorization: Bearer mk_or_rk_key"
//
// The transport is stateless: each POST is one JSON-RPC call. Per request we
// build an MCP server bound to the caller's key. Tool calls run against this
// same server's REST stack via an in-process transport (httpClient) — no socket,
// so tenant isolation, key scopes, quota, and audit are enforced exactly as for
// a direct API call, with no network hop. An optional default agent may be given
// as ?agent_id= or the X-Engram-Agent-Id header (otherwise tools take agent_id
// as a parameter).
//
// Auth is enforced by the APIKeyAuth middleware mounted in front of this route,
// which rejects missing/invalid keys with 401 before we get here.
type MCPHandler struct {
	httpClient *http.Client // backed by the in-process transport; shared, stateless re: key
	baseURL    string       // sentinel host; the in-process transport routes by path
}

// NewMCPHandler builds the embedded MCP endpoint. httpClient must be backed by
// the in-process transport that dispatches to this server's router.
func NewMCPHandler(httpClient *http.Client) *MCPHandler {
	return &MCPHandler{httpClient: httpClient, baseURL: "http://engram.local"}
}

const mcpMaxBodyBytes = 1 << 20 // 1 MiB

// Handle dispatches a single MCP JSON-RPC request.
func (h *MCPHandler) Handle(w http.ResponseWriter, r *http.Request) {
	rawKey := mcpBearer(r.Header.Get("Authorization"))
	if rawKey == "" {
		writeError(w, http.StatusUnauthorized, "missing bearer api key")
		return
	}

	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		agentID = r.Header.Get("X-Engram-Agent-Id")
	}

	// Per-request MCP server bound to the caller's key. The client reuses the
	// shared in-process transport; the key is set per outgoing request, so
	// sharing the underlying *http.Client across tenants is safe.
	client := mcp.NewClientWithHTTP(h.baseURL, rawKey, agentID, h.httpClient)
	server := mcp.NewServer("engram", "0.1.0")
	mcp.RegisterTools(server, client)
	mcp.RegisterResources(server, client)

	var req mcp.Request
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, mcpMaxBodyBytes)).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(mcp.Response{
			JSONRPC: "2.0",
			Error:   &mcp.RPCError{Code: -32700, Message: "parse error"},
		})
		return
	}

	resp := server.Handle(r.Context(), &req)
	if resp == nil { // notification — no response body
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// mcpBearer extracts the token from an "Authorization: Bearer <token>" header.
func mcpBearer(header string) string {
	parts := strings.SplitN(header, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}
