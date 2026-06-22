package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// defaultEmbeddingHTTPTimeout bounds outbound embedding calls so a stalled
// provider can't pin the request goroutine (embeddings run on the memory
// create/recall hot path).
const defaultEmbeddingHTTPTimeout = 60 * time.Second

// CompatibleClient talks to any OpenAI-compatible /embeddings endpoint: a local
// Ollama / vLLM / Text-Embeddings-Inference server, a gateway (LiteLLM), or a
// hosted provider that mirrors OpenAI's wire format. This is what unblocks
// on-prem / data-residency deployments that can't call OpenAI.
type CompatibleClient struct {
	url        string // full /embeddings URL
	apiKey     string // optional (local servers often need none)
	model      string
	dimensions int // when >0, request this output dimension (Matryoshka models)
	httpClient *http.Client
}

// NewCompatibleClient builds a client for an OpenAI-compatible embeddings API.
// baseURL is the server root (e.g. http://localhost:11434/v1); "/embeddings" is
// appended if not already present. dimensions is optional (0 = model default).
func NewCompatibleClient(baseURL, apiKey, model string, dimensions int) *CompatibleClient {
	url := strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(url, "/embeddings") {
		url += "/embeddings"
	}
	if model == "" {
		model = "text-embedding-3-small"
	}
	return &CompatibleClient{
		url:        url,
		apiKey:     apiKey,
		model:      model,
		dimensions: dimensions,
		httpClient: &http.Client{Timeout: defaultEmbeddingHTTPTimeout},
	}
}

type compatibleRequest struct {
	Model      string `json:"model"`
	Input      string `json:"input"`
	Dimensions int    `json:"dimensions,omitempty"`
}

// embeddingResponse is the OpenAI /embeddings response shape, shared by all
// OpenAI-compatible providers.
type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *CompatibleClient) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(compatibleRequest{Model: c.model, Input: text, Dimensions: c.dimensions})
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embedding response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Reuse the OpenAI response shape (data[].embedding).
	var result embeddingResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal embedding response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("embedding API error: %s", result.Error.Message)
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("embedding API returned no data")
	}
	return result.Data[0].Embedding, nil
}
