package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/config"
	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/Harshitk-cp/engram/internal/service"
	"github.com/google/uuid"
)

type AuthHandler struct {
	svc        *service.AuthService
	sessionTTL time.Duration
}

func NewAuthHandler(svc *service.AuthService, sessionTTL time.Duration) *AuthHandler {
	return &AuthHandler{svc: svc, sessionTTL: sessionTTL}
}

func (h *AuthHandler) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   config.CookieSecure(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(h.sessionTTL.Seconds()),
	})
}

func (h *AuthHandler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: middleware.SessionCookieName, Value: "", Path: "/",
		HttpOnly: true, Secure: config.CookieSecure(), SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req credentials
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user, token, err := h.svc.Register(r.Context(), req.Email, req.Password, req.Name)
	if err != nil {
		if errors.Is(err, service.ErrEmailTaken) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.setSessionCookie(w, token)
	writeJSON(w, http.StatusCreated, map[string]any{"user": userView(user)})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req credentials
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	token, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	h.setSessionCookie(w, token)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(middleware.SessionCookieName); err == nil {
		_ = h.svc.Logout(r.Context(), c.Value)
	}
	h.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(middleware.SessionCookieName)
	if err != nil || c.Value == "" {
		writeError(w, http.StatusUnauthorized, "not signed in")
		return
	}
	user, sess, orgs, err := h.svc.CurrentUser(r.Context(), c.Value)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "session invalid")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":             userView(user),
		"active_tenant_id": sess.ActiveTenantID,
		"orgs":             orgs,
	})
}

func (h *AuthHandler) SwitchTenant(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(middleware.SessionCookieName)
	if err != nil || c.Value == "" {
		writeError(w, http.StatusUnauthorized, "not signed in")
		return
	}
	var req struct {
		TenantID string `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	tid, err := uuid.Parse(req.TenantID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tenant_id")
		return
	}
	if err := h.svc.SwitchTenant(r.Context(), c.Value, tid); err != nil {
		writeError(w, http.StatusForbidden, "not a member of that org")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CreateOrg provisions a new organization for the signed-in user and switches
// the session to it.
func (h *AuthHandler) CreateOrg(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(middleware.SessionCookieName)
	if err != nil || c.Value == "" {
		writeError(w, http.StatusUnauthorized, "not signed in")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	org, err := h.svc.CreateOrg(r.Context(), c.Value, req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, org)
}

// Config tells the console which auth methods are available.
func (h *AuthHandler) Config(w http.ResponseWriter, r *http.Request) {
	gid, _ := config.GoogleOAuth()
	ghid, _ := config.GitHubOAuth()
	wid, wkey := config.WorkOSAuth()
	writeJSON(w, http.StatusOK, map[string]any{
		"password": true,
		"google":   gid != "",
		"github":   ghid != "",
		"workos":   wid != "" && wkey != "",
	})
}

func userView(u *domain.User) map[string]any {
	return map[string]any{"id": u.ID, "email": u.Email, "name": u.Name, "avatar_url": u.AvatarURL}
}
