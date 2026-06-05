// engram-mcp is the Model Context Protocol server for Engram.
//
// Usage:
//
//	engram-mcp [--transport stdio|sse|http] [--addr :3742]
//
// Environment variables:
//
//	ENGRAM_API_URL   Engram server URL (default: http://localhost:8080)
//	ENGRAM_API_KEY   API key (mk_... or rk_...)
//	ENGRAM_AGENT_ID  Default agent ID used when a tool call omits agent_id
//
// Claude Desktop config (~/.claude/claude_desktop_config.json):
//
//	{
//	  "mcpServers": {
//	    "engram": {
//	      "command": "engram-mcp",
//	      "args": ["--transport", "stdio"],
//	      "env": {
//	        "ENGRAM_API_URL": "http://localhost:8080",
//	        "ENGRAM_API_KEY": "mk_...",
//	        "ENGRAM_AGENT_ID": "..."
//	      }
//	    }
//	  }
//	}
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/Harshitk-cp/engram/mcp"
	"github.com/Harshitk-cp/engram/mcp/transports"
)

func main() {
	transport := flag.String("transport", "stdio", "Transport: stdio, sse, or http")
	addr := flag.String("addr", ":3742", "Listen address for sse and http transports")
	flag.Parse()

	apiURL := envOr("ENGRAM_API_URL", "http://localhost:8080")
	apiKey := os.Getenv("ENGRAM_API_KEY")
	agentID := os.Getenv("ENGRAM_AGENT_ID")

	if apiKey == "" {
		log.Fatal("ENGRAM_API_KEY is required")
	}

	client := mcp.NewClient(apiURL, apiKey, agentID)

	server := mcp.NewServer("engram", "0.1.0")
	mcp.RegisterTools(server, client)
	mcp.RegisterResources(server, client)

	ctx := context.Background()

	switch *transport {
	case "stdio":
		log.SetOutput(os.Stderr) // keep stderr for diagnostics; stdout is the MCP channel
		log.Printf("engram-mcp started (stdio, api_url=%s, agent_id=%q)", apiURL, agentID)
		if err := transports.Stdio(ctx, server); err != nil {
			log.Fatalf("stdio transport error: %v", err)
		}

	case "sse":
		sseServer := transports.NewSSEServer(server)
		log.Printf("engram-mcp SSE transport listening on %s", *addr)
		if err := http.ListenAndServe(*addr, sseServer.Handler()); err != nil {
			log.Fatalf("SSE server error: %v", err)
		}

	case "http":
		httpServer := transports.NewHTTPServer(server)
		log.Printf("engram-mcp HTTP transport listening on %s", *addr)
		if err := http.ListenAndServe(*addr, httpServer.Handler()); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}

	default:
		fmt.Fprintf(os.Stderr, "unknown transport %q — use stdio, sse, or http\n", *transport)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
