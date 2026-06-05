package mcp

import "encoding/json"

const jsonrpcVersion = "2.0"

// JSON-RPC 2.0 error codes
const (
	errParseError     = -32700
	errInvalidRequest = -32600
	errMethodNotFound = -32601
	errInvalidParams  = -32602
	errInternal       = -32603
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (r *Request) IsNotification() bool {
	return len(r.ID) == 0
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newResponse(id json.RawMessage, result interface{}) *Response {
	return &Response{JSONRPC: jsonrpcVersion, ID: id, Result: result}
}

func newErrorResponse(id json.RawMessage, code int, msg string) *Response {
	return &Response{JSONRPC: jsonrpcVersion, ID: id, Error: &RPCError{Code: code, Message: msg}}
}
