package transports

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Harshitk-cp/engram/mcp"
)

// HTTPServer implements the MCP Streamable HTTP transport (single endpoint).
//
//	POST /mcp — accepts a JSON-RPC request, returns a JSON-RPC response.
//
// This is the simplest HTTP integration and works with any HTTP client.
// For streaming (batch) use the SSE transport instead.
type HTTPServer struct {
	server *mcp.Server
}

// NewHTTPServer creates an HTTPServer wrapping the given MCP server.
func NewHTTPServer(server *mcp.Server) *HTTPServer {
	return &HTTPServer{server: server}
}

// Handler returns an http.Handler that mounts the HTTP transport.
func (s *HTTPServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", s.handleMCP)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","server":"engram-mcp"}`))
	})
	return mux
}

func (s *HTTPServer) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req mcp.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp := mcp.Response{
			JSONRPC: "2.0",
			Error:   &mcp.RPCError{Code: -32700, Message: "parse error"},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp := s.server.Handle(context.Background(), &req)
	if resp == nil {
		// Notification: return 204
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
