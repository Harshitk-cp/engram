package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/store"
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
	valid, checked, breakSeq, err := h.store.VerifyChain(r.Context(), tenant.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "verify failed")
		return
	}
	headSeq, headHash, _ := h.store.ChainHead(r.Context(), tenant.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"valid":       valid,
		"checked":     checked,
		"break_seq":   breakSeq,
		"head_seq":    headSeq,
		"head_hash":   headHash,
		"signed":      h.signingKey != "",
		"verified_at": time.Now().UTC(),
	})
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
