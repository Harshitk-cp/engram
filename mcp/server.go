// Package mcp implements the Model Context Protocol server for Engram.
// It exposes Engram's memory operations as MCP tools and resources,
// enabling integration with Claude Desktop, Cursor, Windsurf, and any MCP host.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// Tool is an MCP tool definition with its JSON Schema input description.
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

// CallToolResult is the response from a tool invocation.
type CallToolResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ToolContent is a single piece of content in a tool result.
type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Resource is an MCP resource definition.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourceContent is the content of a resource.
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

// ToolHandler is a function that handles a tool call.
type ToolHandler func(ctx context.Context, args map[string]interface{}) (*CallToolResult, error)

// ResourceHandler is a function that handles a resource read.
// The uri argument may be a template pattern — the handler receives the resolved URI.
type ResourceHandler func(ctx context.Context, uri string) (*ResourceContent, error)

// resourceEntry pairs a resource definition with its handler and URI template.
type resourceEntry struct {
	resource Resource
	handler  ResourceHandler
	// uriTemplate is the URI template pattern (e.g. "engram://agents/{agent_id}/memories")
	uriTemplate string
}

// Server is the MCP protocol handler. It is transport-agnostic: the transports
// call Handle() with a decoded request and send back the returned response.
type Server struct {
	name      string
	version   string
	tools     []Tool
	toolMap   map[string]ToolHandler
	resources []resourceEntry
}

func NewServer(name, version string) *Server {
	return &Server{
		name:    name,
		version: version,
		toolMap: make(map[string]ToolHandler),
	}
}

// AddTool registers an MCP tool.
func (s *Server) AddTool(tool Tool, handler ToolHandler) {
	s.tools = append(s.tools, tool)
	s.toolMap[tool.Name] = handler
}

// AddResource registers an MCP resource with its URI template.
func (s *Server) AddResource(resource Resource, uriTemplate string, handler ResourceHandler) {
	s.resources = append(s.resources, resourceEntry{
		resource:    resource,
		handler:     handler,
		uriTemplate: uriTemplate,
	})
}

// Handle dispatches a JSON-RPC request and returns the response.
// Returns nil for notifications (no ID) that require no response.
func (s *Server) Handle(ctx context.Context, req *Request) *Response {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized", "notifications/cancelled":
		return nil // notifications require no response
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	case "resources/list":
		return s.handleResourcesList(req)
	case "resources/read":
		return s.handleResourcesRead(ctx, req)
	case "ping":
		return newResponse(req.ID, map[string]interface{}{})
	default:
		return newErrorResponse(req.ID, errMethodNotFound, fmt.Sprintf("method not found: %s", req.Method))
	}
}

func (s *Server) handleInitialize(req *Request) *Response {
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools":     map[string]interface{}{},
			"resources": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    s.name,
			"version": s.version,
		},
	}
	return newResponse(req.ID, result)
}

func (s *Server) handleToolsList(req *Request) *Response {
	tools := s.tools
	if tools == nil {
		tools = []Tool{}
	}
	return newResponse(req.ID, map[string]interface{}{"tools": tools})
}

func (s *Server) handleToolsCall(ctx context.Context, req *Request) *Response {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return newErrorResponse(req.ID, errInvalidParams, "invalid params")
	}

	handler, ok := s.toolMap[params.Name]
	if !ok {
		return newErrorResponse(req.ID, errMethodNotFound, fmt.Sprintf("tool not found: %s", params.Name))
	}

	result, err := handler(ctx, params.Arguments)
	if err != nil {
		result = &CallToolResult{
			Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Error: %s", err.Error())}},
			IsError: true,
		}
	}
	return newResponse(req.ID, result)
}

func (s *Server) handleResourcesList(req *Request) *Response {
	resources := make([]Resource, len(s.resources))
	for i, e := range s.resources {
		resources[i] = e.resource
	}
	return newResponse(req.ID, map[string]interface{}{"resources": resources})
}

func (s *Server) handleResourcesRead(ctx context.Context, req *Request) *Response {
	var params struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return newErrorResponse(req.ID, errInvalidParams, "invalid params")
	}

	for _, entry := range s.resources {
		if matchURITemplate(entry.uriTemplate, params.URI) {
			content, err := entry.handler(ctx, params.URI)
			if err != nil {
				return newErrorResponse(req.ID, errInternal, err.Error())
			}
			return newResponse(req.ID, map[string]interface{}{
				"contents": []ResourceContent{*content},
			})
		}
	}
	return newErrorResponse(req.ID, errInvalidParams, fmt.Sprintf("resource not found: %s", params.URI))
}

// matchURITemplate checks if uri matches a template like "engram://agents/{agent_id}/memories".
// Simple implementation: matches if prefix and suffix around {param} match.
func matchURITemplate(template, uri string) bool {
	// Find the first { in the template
	for i := 0; i < len(template); i++ {
		if template[i] == '{' {
			end := i
			for end < len(template) && template[end] != '}' {
				end++
			}
			suffix := template[end+1:]
			prefix := template[:i]
			return len(uri) >= len(prefix)+len(suffix) &&
				uri[:len(prefix)] == prefix &&
				uri[len(uri)-len(suffix):] == suffix
		}
	}
	return template == uri
}

// TextResult is a convenience constructor for a successful text tool result.
func TextResult(text string) *CallToolResult {
	return &CallToolResult{Content: []ToolContent{{Type: "text", Text: text}}}
}

// ErrorResult is a convenience constructor for an error tool result.
func ErrorResult(text string) *CallToolResult {
	return &CallToolResult{Content: []ToolContent{{Type: "text", Text: text}}, IsError: true}
}
