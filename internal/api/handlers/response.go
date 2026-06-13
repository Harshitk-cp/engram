package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/store"
	"github.com/google/uuid"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func requireAgentInTenant(w http.ResponseWriter, r *http.Request, agents domain.AgentStore, agentID, tenantID uuid.UUID) bool {
	agent, err := agents.GetByID(r.Context(), agentID, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "agent not found")
		} else {
			writeError(w, http.StatusInternalServerError, "failed to verify agent")
		}
		return false
	}
	if agent == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return false
	}
	return true
}

func requireMemoryInTenant(w http.ResponseWriter, r *http.Request, memories domain.MemoryStore, memoryID, tenantID uuid.UUID) bool {
	mem, err := memories.GetByID(r.Context(), memoryID, tenantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "memory not found")
		} else {
			writeError(w, http.StatusInternalServerError, "failed to verify memory")
		}
		return false
	}
	if mem == nil {
		writeError(w, http.StatusNotFound, "memory not found")
		return false
	}
	return true
}
