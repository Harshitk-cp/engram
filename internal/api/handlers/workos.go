package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/Harshitk-cp/engram/internal/config"
)

// WorkOS enterprise SSO via AuthKit. The user_management flow gives a hosted
// login where the user enters their email and WorkOS routes them to their org's
// IdP (SAML/OIDC) — no per-connection wiring needed in the console.
const (
	workosAuthorizeURL    = "https://api.workos.com/user_management/authorize"
	workosAuthenticateURL = "https://api.workos.com/user_management/authenticate"
)

// SSOStart handles GET /auth/sso/start — redirect to WorkOS AuthKit.
func (h *AuthHandler) SSOStart(w http.ResponseWriter, r *http.Request) {
	clientID, apiKey := config.WorkOSAuth()
	if clientID == "" || apiKey == "" {
		http.Redirect(w, r, "/login?error=provider_not_configured", http.StatusFound)
		return
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	state := hex.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{Name: "sso_state", Value: state, Path: "/", HttpOnly: true,
		Secure: config.CookieSecure(), SameSite: http.SameSiteLaxMode, MaxAge: 600})

	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("redirect_uri", config.AppBaseURL()+"/auth/sso/callback")
	q.Set("response_type", "code")
	q.Set("provider", "authkit")
	q.Set("state", state)
	http.Redirect(w, r, workosAuthorizeURL+"?"+q.Encode(), http.StatusFound)
}

// SSOCallback handles GET /auth/sso/callback — exchange the code with WorkOS,
// which returns the (IdP-verified) user, then start a session.
func (h *AuthHandler) SSOCallback(w http.ResponseWriter, r *http.Request) {
	clientID, apiKey := config.WorkOSAuth()
	if clientID == "" || apiKey == "" {
		http.Redirect(w, r, "/login?error=provider_not_configured", http.StatusFound)
		return
	}
	stateCookie, err := r.Cookie("sso_state")
	if err != nil || stateCookie.Value == "" || stateCookie.Value != r.URL.Query().Get("state") {
		http.Redirect(w, r, "/login?error=state_mismatch", http.StatusFound)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Redirect(w, r, "/login?error=no_code", http.StatusFound)
		return
	}

	body, _ := json.Marshal(map[string]string{
		"client_id":     clientID,
		"client_secret": apiKey,
		"grant_type":    "authorization_code",
		"code":          code,
	})
	req, _ := http.NewRequest(http.MethodPost, workosAuthenticateURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		http.Redirect(w, r, "/login?error=token_exchange", http.StatusFound)
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out struct {
		User struct {
			ID                string `json:"id"`
			Email             string `json:"email"`
			FirstName         string `json:"first_name"`
			LastName          string `json:"last_name"`
			ProfilePictureURL string `json:"profile_picture_url"`
		} `json:"user"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.User.ID == "" {
		http.Redirect(w, r, "/login?error=profile", http.StatusFound)
		return
	}
	name := out.User.FirstName
	if out.User.LastName != "" {
		name = name + " " + out.User.LastName
	}

	// WorkOS asserts the email via the org's IdP, so it is verified — safe to link.
	token, err := h.svc.OAuthLogin(r.Context(), "workos", out.User.ID, out.User.Email, name, out.User.ProfilePictureURL)
	if err != nil {
		http.Redirect(w, r, "/login?error=login_failed", http.StatusFound)
		return
	}
	h.setSessionCookie(w, token)
	http.Redirect(w, r, "/", http.StatusFound)
}
