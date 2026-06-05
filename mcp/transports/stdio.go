// Package transports implements MCP transport layers: stdio, SSE, and HTTP.
package transports

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/Harshitk-cp/engram/mcp"
)

// This is the transport used by Claude Desktop and most local MCP hosts.
func Stdio(ctx context.Context, server *mcp.Server) error {
	return StdioRW(ctx, server, os.Stdin, os.Stdout)
}

// StdioRW is the testable core of Stdio — same protocol, explicit reader/writer.
func StdioRW(ctx context.Context, server *mcp.Server, r io.Reader, w io.Writer) error {
	encoder := json.NewEncoder(w)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024) // 4 MB max message

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req mcp.Request
		if err := json.Unmarshal(line, &req); err != nil {
			resp := mcp.Response{
				JSONRPC: "2.0",
				Error:   &mcp.RPCError{Code: -32700, Message: fmt.Sprintf("parse error: %s", err)},
			}
			_ = encoder.Encode(resp)
			continue
		}

		resp := server.Handle(ctx, &req)
		if resp == nil {
			continue // notification — no response needed
		}
		if err := encoder.Encode(resp); err != nil {
			return fmt.Errorf("stdio: encode response: %w", err)
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return fmt.Errorf("stdio: read: %w", err)
	}
	return nil
}
