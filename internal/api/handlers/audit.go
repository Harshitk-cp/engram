package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/google/uuid"
)

type AuditHandler struct {
	store      *store.MutationLogStore
	signingKey string
}

func NewAuditHandler(s *store.MutationLogStore, signingKey string) *AuditHandler {
	return &AuditHandler{store: s, signingKey: signingKey}
}

// Verify handles GET /v1/audit/verify — recomputes the tenant's hash chain and
// reports whether the audit trail is intact (tamper-evident).
func (h *AuditHandler) Verify(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	valid, checked, breakSeq, reason, err := h.store.VerifyChain(r.Context(), tenant.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "verify failed")
		return
	}
	headSeq, headHash, _ := h.store.ChainHead(r.Context(), tenant.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"valid":       valid,
		"checked":     checked,
		"break_seq":   breakSeq,
		"reason":      reason,
		"head_seq":    headSeq,
		"head_hash":   headHash,
		"signed":      h.signingKey != "",
		"verified_at": time.Now().UTC(),
	})
}

// Chain handles GET /v1/audit/chain — returns recent hash-chain rows (with
// seq/prev_hash/row_hash) for visualizing the audit trail in the console.
// Optional ?agent_id= filters to one agent's records within the tenant chain;
// ?limit= caps how many (default 100, max 500).
func (h *AuditHandler) Chain(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var agentID *uuid.UUID
	if s := r.URL.Query().Get("agent_id"); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid agent_id")
			return
		}
		agentID = &id
	}
	limit := 100
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			limit = n
		}
	}
	entries, err := h.store.ChainEntries(r.Context(), tenant.ID, agentID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load chain")
		return
	}
	if entries == nil {
		entries = []domain.MutationLog{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "count": len(entries)})
}

// Export handles GET /v1/audit/export — streams the tenant's full audit trail as
// NDJSON, with a trailer carrying the head hash and (if configured) an HMAC so a
// recipient can confirm origin and completeness.
func (h *AuditHandler) Export(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	records, err := h.store.ExportByTenant(r.Context(), tenant.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "export failed")
		return
	}
	headSeq, headHash, _ := h.store.ChainHead(r.Context(), tenant.ID)

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=engram-audit-%s.ndjson", tenant.ID))
	enc := json.NewEncoder(w)
	for i := range records {
		_ = enc.Encode(records[i])
	}
	trailer := map[string]any{
		"_trailer":    true,
		"tenant_id":   tenant.ID,
		"count":       len(records),
		"head_seq":    headSeq,
		"head_hash":   headHash,
		"exported_at": time.Now().UTC(),
	}
	if h.signingKey != "" {
		mac := hmac.New(sha256.New, []byte(h.signingKey))
		mac.Write([]byte(headHash))
		trailer["signature"] = hex.EncodeToString(mac.Sum(nil))
		trailer["signature_alg"] = "HMAC-SHA256(head_hash)"
	}
	_ = enc.Encode(trailer)
}
