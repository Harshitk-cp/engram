package api

import (
	"context"
	"io"
	"net/http"
	"testing"

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
