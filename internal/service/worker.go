package service

import (
	"runtime/debug"

	"go.uber.org/zap"
)

// guardPanic runs fn and converts a panic into an error log. Background
// workers iterate every tenant's data; without this, one corrupt row or
// malformed embedding panicking a tick would crash the whole server.
func guardPanic(logger *zap.Logger, what string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("panic recovered in background worker",
				zap.String("in", what),
				zap.Any("panic", r),
				zap.ByteString("stack", debug.Stack()))
		}
	}()
	fn()
}
