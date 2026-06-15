package handlers

import (
	"net/http"

	"github.com/Harshitk-cp/engram/internal/config"
)

// EmbeddingHandler exposes the deployment's active embedding configuration
// (read-only). The model/dimension are a deploy-time choice shared by all
// tenants, so this is informational, not editable.
type EmbeddingHandler struct{}

func NewEmbeddingHandler() *EmbeddingHandler { return &EmbeddingHandler{} }

// Info handles GET /v1/embedding/info.
func (h *EmbeddingHandler) Info(w http.ResponseWriter, r *http.Request) {
	model := config.EmbeddingModel()
	if model == "" {
		model = "text-embedding-3-small (provider default)"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"provider":  config.EmbeddingProvider(),
		"model":     model,
		"dimension": config.EmbeddingDim(),
		"base_url":  config.EmbeddingBaseURL(),
	})
}
