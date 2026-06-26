package api

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	mw "github.com/Harshitk-cp/engram/internal/api/middleware"
)

// The in-process transport must dispatch to the handler, capture status + body,
// and tag the request internal so the rate limiter can skip it.
func TestInProcessRoundTripper(t *testing.T) {
	var sawInternal bool
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawInternal = mw.IsInternal(r.Context())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	rt := inProcessRoundTripper{handler: h}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://engram.local/v1/x", nil)

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("roundtrip: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"ok":true}` {
		t.Fatalf("body = %q", body)
	}
	if !sawInternal {
		t.Fatalf("dispatched request was not tagged internal")
	}
}

// Regression: the embedded /mcp handler runs inside chi and threads its request
// context (carrying the outer POST /mcp RouteContext) into in-process tool calls.
// chi.Mux reuses an existing RouteContext, so a GET tool call would be matched as
// POST against the parent's path — GET-only routes 405 and POST-sibling routes
// 500. The transport must reset the RouteContext so each call routes on its own
// method + path. This builds a chi router shaped like the real one and dispatches
// a GET through the transport from inside a POST /mcp match.
func TestInProcessRoundTripper_ResetsChiRouteContext(t *testing.T) {
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Route("/memories", func(r chi.Router) {
			r.Get("/recall", func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"results":[]}`))
			})
		})
		r.Route("/agents", func(r chi.Router) {
			r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"agents":[]}`))
			})
			// A POST sibling: if a GET is mis-routed as POST it lands here.
			r.Post("/", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			})
		})
	})

	rt := inProcessRoundTripper{handler: r}

	// Simulate the request originating inside the POST /mcp match: prime the
	// context with a chi RouteContext whose method is POST, exactly as chi would
	// have left it after routing the outer /mcp call.
	parent := chi.NewRouteContext()
	parent.RouteMethod = http.MethodPost
	parentCtx := context.WithValue(context.Background(), chi.RouteCtxKey, parent)

	for _, tc := range []struct {
		name, path string
	}{
		{"GET recall (GET-only route)", "/v1/memories/recall"},
		{"GET list-agents (has POST sibling)", "/v1/agents/"},
	} {
		req, _ := http.NewRequestWithContext(parentCtx, http.MethodGet, "http://engram.local"+tc.path, nil)
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatalf("%s: roundtrip: %v", tc.name, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200 (stale parent RouteContext leaked the POST method)", tc.name, resp.StatusCode)
		}
	}
}
