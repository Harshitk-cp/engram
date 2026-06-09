package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client is an HTTP client for the Engram API.
type Client struct {
	baseURL  string
	apiKey   string
	agentID  string
	http     *http.Client
}

// NewClient creates an Engram API client.
// agentID is the default agent used when the caller does not supply one.
func NewClient(baseURL, apiKey, agentID string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		agentID: agentID,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) AgentID() string { return c.agentID }

// MemoryResult is returned by Remember.
type MemoryResult struct {
	ID         string  `json:"id"`
	Content    string  `json:"content"`
	Type       string  `json:"type"`
	Confidence float64 `json:"confidence"`
	Tier       string  `json:"tier"`
	TierReason string  `json:"tier_reason"`
	Reinforced bool    `json:"reinforced"`
}

// RecallMemory is a single memory in a recall result.
type RecallMemory struct {
	ID         string  `json:"id"`
	Content    string  `json:"content"`
	Type       string  `json:"type"`
	Confidence float64 `json:"confidence"`
	Score      float64 `json:"score"`
	Source     string  `json:"source"`
}

// AgentResult is returned by ListAgents.
type AgentResult struct {
	ID         string `json:"id"`
	ExternalID string `json:"external_id"`
	Name       string `json:"name"`
	CreatedAt  string `json:"created_at"`
}

// Remember stores a memory in Engram. confidence=0 lets the server assign the
// default. anchor is the caller's own id for who/what the memory is about; when
// set, the trace is bound to that anchor (which must already exist).
func (c *Client) Remember(ctx context.Context, agentID, content, memType, source, anchor, session string, confidence float32) (*MemoryResult, error) {
	if agentID == "" {
		agentID = c.agentID
	}
	body := map[string]interface{}{
		"agent_id": agentID,
		"content":  content,
		"source":   source,
	}
	// `source` carries the belief's origin (user/agent/inferred/tool); send it as
	// provenance too so the server records it and derives the initial confidence.
	if source != "" {
		body["provenance"] = source
	}
	if memType != "" {
		body["type"] = memType
	}
	if confidence > 0 {
		body["confidence"] = confidence
	}
	if anchor != "" {
		body["anchor_external_id"] = anchor
	}
	if session != "" {
		body["session_id"] = session
	}

	var result MemoryResult
	if err := c.post(ctx, "/v1/memories", body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Recall retrieves memories matching a query. anchor (the caller's own id for a
// subject) restricts recall to traces about that anchor.
func (c *Client) Recall(ctx context.Context, agentID, query string, topK int, graphWeight float64, anchor, session string) ([]RecallMemory, error) {
	if agentID == "" {
		agentID = c.agentID
	}
	if topK <= 0 {
		topK = 10
	}

	params := url.Values{}
	params.Set("agent_id", agentID)
	params.Set("query", query)
	params.Set("top_k", strconv.Itoa(topK))
	if graphWeight > 0 {
		params.Set("graph_weight", strconv.FormatFloat(graphWeight, 'f', 2, 64))
	}
	if anchor != "" {
		params.Set("anchor_external_id", anchor)
	}
	if session != "" {
		params.Set("session_id", session)
	}

	var resp struct {
		Memories []RecallMemory `json:"memories"`
		Results  []RecallMemory `json:"results"`
	}
	if err := c.get(ctx, "/v1/memories/recall?"+params.Encode(), &resp); err != nil {
		return nil, err
	}
	if resp.Memories != nil {
		return resp.Memories, nil
	}
	return resp.Results, nil
}

// Forget deletes a memory by ID.
func (c *Client) Forget(ctx context.Context, memoryID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/v1/memories/"+memoryID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("engram API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("engram API: status %d: %s", resp.StatusCode, body)
}

// ListAgents returns the agents for the authenticated tenant.
func (c *Client) ListAgents(ctx context.Context) ([]AgentResult, error) {
	var resp struct {
		Agents []AgentResult `json:"agents"`
	}
	if err := c.get(ctx, "/v1/agents", &resp); err != nil {
		return nil, err
	}
	return resp.Agents, nil
}

// GetHotMemories returns the hot-tier memories for an agent.
func (c *Client) GetHotMemories(ctx context.Context, agentID string, limit int) ([]RecallMemory, error) {
	if agentID == "" {
		agentID = c.agentID
	}
	if limit <= 0 {
		limit = 10
	}
	params := url.Values{}
	params.Set("limit", strconv.Itoa(limit))

	var resp struct {
		Memories []RecallMemory `json:"memories"`
	}
	path := fmt.Sprintf("/v1/agents/%s/hot-memories?%s", agentID, params.Encode())
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Memories, nil
}

func (c *Client) post(ctx context.Context, path string, body interface{}, result interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("engram API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("engram API: status %d: %s", resp.StatusCode, respBody)
	}
	return json.Unmarshal(respBody, result)
}

func (c *Client) get(ctx context.Context, path string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("engram API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("engram API: status %d: %s", resp.StatusCode, body)
	}
	return json.Unmarshal(body, result)
}
