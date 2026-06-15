package transports

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/Harshitk-cp/engram/mcp"
)

// SSEServer implements the MCP SSE transport:
//
//	GET  /sse      — client connects; server streams JSON-RPC responses as SSE events
//	POST /message  — client posts JSON-RPC requests
//
// Each connection gets its own channel. The server fan-outs responses back to
// the originating SSE connection via a session map keyed by a session token
// passed as ?sessionId= on the POST endpoint.
type SSEServer struct {
	server  *mcp.Server
	mu      sync.Mutex
	session map[string]chan []byte // sessionID → response channel
}

// NewSSEServer creates an SSEServer wrapping the given MCP server.
func NewSSEServer(server *mcp.Server) *SSEServer {
	return &SSEServer{
		server:  server,
		session: make(map[string]chan []byte),
	}
}

// Handler returns an http.Handler that mounts the SSE transport.
func (s *SSEServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/sse", s.handleSSE)
	mux.HandleFunc("/message", s.handleMessage)
	return mux
}

func (s *SSEServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	sessionID := generateID()
	ch := make(chan []byte, 32)

	s.mu.Lock()
	s.session[sessionID] = ch
	s.mu.Unlock()

	// close(ch) must happen under the same lock that guards sends in
	// handleMessage, otherwise a POST racing a disconnect panics the process
	// with a send on a closed channel.
	defer func() {
		s.mu.Lock()
		delete(s.session, sessionID)
		close(ch)
		s.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Send the endpoint URL as the first event so the client knows where to POST.
	endpoint := fmt.Sprintf("/message?sessionId=%s", sessionID)
	fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", endpoint)
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, open := <-ch:
			if !open {
				return
			}
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

func (s *SSEServer) handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.URL.Query().Get("sessionId")

	var req mcp.Request
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	resp := s.server.Handle(r.Context(), &req)
	if resp == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	data, _ := json.Marshal(resp)

	// If we have a session, fan the response to the SSE stream.
	if sessionID != "" {
		s.mu.Lock()
		ch, ok := s.session[sessionID]
		if ok {
			select {
			case ch <- data:
			default: // drop if the channel is full
			}
		}
		s.mu.Unlock()

		if ok {
			w.WriteHeader(http.StatusAccepted)
			return
		}
	}

	// Fallback: return the response directly in the HTTP body.
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand unavailable: %v", err))
	}
	return hex.EncodeToString(b)
}
