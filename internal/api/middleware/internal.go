package middleware

import "context"

type internalCallKeyType struct{}

var internalCallKey internalCallKeyType

// WithInternal marks ctx as an in-process call originating inside this server —
// e.g. the embedded MCP endpoint dispatching to the REST stack. Context values
// can't be set by external HTTP clients, so middleware can trust this flag and
// treat the request differently (the rate limiter skips it, since the external
// request that triggered it was already limited at the edge).
func WithInternal(ctx context.Context) context.Context {
	return context.WithValue(ctx, internalCallKey, true)
}

// IsInternal reports whether ctx was tagged by WithInternal.
func IsInternal(ctx context.Context) bool {
	v, _ := ctx.Value(internalCallKey).(bool)
	return v
}
