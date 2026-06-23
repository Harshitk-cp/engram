package api

import (
	"bytes"
	"io"
	"net/http"

	mw "github.com/Harshitk-cp/engram/internal/api/middleware"
)

// inProcessRoundTripper dispatches an HTTP request to a handler in the same
// process — no socket, no port, no connection pool. The embedded MCP endpoint
// uses it so MCP tool calls reuse the full REST stack (auth, scope, quota,
// audit) without a network hop, while staying a thin facade over the same API
// rather than a second, drift-prone copy of the handler logic.
//
// Each dispatched request is tagged internal so the rate limiter skips it: the
// inbound request that triggered it was already limited at the edge, and the
// internal fan-out is bounded by that.
type inProcessRoundTripper struct {
	handler http.Handler
}

func (rt inProcessRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := &responseCapture{header: make(http.Header), body: new(bytes.Buffer)}
	rt.handler.ServeHTTP(rec, req.WithContext(mw.WithInternal(req.Context())))

	status := rec.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode:    status,
		Status:        http.StatusText(status),
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        rec.header,
		Body:          io.NopCloser(rec.body),
		ContentLength: int64(rec.body.Len()),
		Request:       req,
	}, nil
}

// responseCapture is a minimal in-memory http.ResponseWriter for in-process
// dispatch. The /v1 endpoints the MCP tools call write a status + JSON body and
// don't stream, so Flusher/Hijacker aren't needed.
type responseCapture struct {
	status int
	header http.Header
	body   *bytes.Buffer
}

func (rc *responseCapture) Header() http.Header { return rc.header }

func (rc *responseCapture) WriteHeader(code int) {
	if rc.status == 0 {
		rc.status = code
	}
}

func (rc *responseCapture) Write(b []byte) (int, error) {
	if rc.status == 0 {
		rc.status = http.StatusOK
	}
	return rc.body.Write(b)
}
