package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Harshitk-cp/engram/internal/config"
	"github.com/go-chi/chi/v5"
)

type oauthProvider struct {
	clientID, clientSecret                string
	authURL, tokenURL, userInfoURL, scope string
}

func providerConf(provider string) (oauthProvider, bool) {
	switch provider {
	case "google":
		id, secret := config.GoogleOAuth()
		if id == "" || secret == "" {
			return oauthProvider{}, false
		}
		return oauthProvider{id, secret,
			"https://accounts.google.com/o/oauth2/v2/auth",
			"https://oauth2.googleapis.com/token",
			"https://www.googleapis.com/oauth2/v3/userinfo",
			"openid email profile"}, true
	case "github":
		id, secret := config.GitHubOAuth()
		if id == "" || secret == "" {
			return oauthProvider{}, false
		}
		return oauthProvider{id, secret,
			"https://github.com/login/oauth/authorize",
			"https://github.com/login/oauth/access_token",
			"https://api.github.com/user",
			"read:user user:email"}, true
	}
	return oauthProvider{}, false
}

func (h *AuthHandler) redirectURI(provider string) string {
	return config.AppBaseURL() + "/auth/oauth/" + provider + "/callback"
}

// OAuthStart redirects the browser to the provider's consent screen.
func (h *AuthHandler) OAuthStart(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	conf, ok := providerConf(provider)
	if !ok {
		http.Redirect(w, r, "/login?error=provider_not_configured", http.StatusFound)
		return
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	state := hex.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{Name: "oauth_state", Value: state, Path: "/", HttpOnly: true,
		Secure: config.CookieSecure(), SameSite: http.SameSiteLaxMode, MaxAge: 600})

	q := url.Values{}
	q.Set("client_id", conf.clientID)
	q.Set("redirect_uri", h.redirectURI(provider))
	q.Set("response_type", "code")
	q.Set("scope", conf.scope)
	q.Set("state", state)
	http.Redirect(w, r, conf.authURL+"?"+q.Encode(), http.StatusFound)
}

// OAuthCallback completes the flow: validates state, exchanges the code, fetches
// the profile, logs the user in, and redirects to the console.
func (h *AuthHandler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	conf, ok := providerConf(provider)
	if !ok {
		http.Redirect(w, r, "/login?error=provider_not_configured", http.StatusFound)
		return
	}
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value == "" || stateCookie.Value != r.URL.Query().Get("state") {
		http.Redirect(w, r, "/login?error=state_mismatch", http.StatusFound)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Redirect(w, r, "/login?error=no_code", http.StatusFound)
		return
	}

	accessToken, err := h.exchangeCode(conf, provider, code)
	if err != nil {
		http.Redirect(w, r, "/login?error=token_exchange", http.StatusFound)
		return
	}
	providerUserID, email, name, avatar, err := h.fetchProfile(conf, provider, accessToken)
	if err != nil || providerUserID == "" {
		http.Redirect(w, r, "/login?error=profile", http.StatusFound)
		return
	}

	token, err := h.svc.OAuthLogin(r.Context(), provider, providerUserID, email, name, avatar)
	if err != nil {
		http.Redirect(w, r, "/login?error=login_failed", http.StatusFound)
		return
	}
	h.setSessionCookie(w, token)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *AuthHandler) exchangeCode(conf oauthProvider, provider, code string) (string, error) {
	form := url.Values{}
	form.Set("client_id", conf.clientID)
	form.Set("client_secret", conf.clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", h.redirectURI(provider))
	form.Set("grant_type", "authorization_code")

	req, _ := http.NewRequest(http.MethodPost, conf.tokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return "", fmt.Errorf("no access token")
	}
	return tok.AccessToken, nil
}

func (h *AuthHandler) fetchProfile(conf oauthProvider, provider, accessToken string) (id, email, name, avatar string, err error) {
	prof, err := getJSON(conf.userInfoURL, accessToken)
	if err != nil {
		return "", "", "", "", err
	}
	switch provider {
	case "google":
		id, _ = prof["sub"].(string)
		name, _ = prof["name"].(string)
		avatar, _ = prof["picture"].(string)
		// SECURITY: only surface the email when Google attests it is verified, so
		// downstream account linking can't be hijacked with an unverified address.
		if googleEmailVerified(prof) {
			email, _ = prof["email"].(string)
		}
	case "github":
		if v, ok := prof["id"].(float64); ok {
			id = fmt.Sprintf("%d", int64(v))
		}
		name, _ = prof["name"].(string)
		if name == "" {
			name, _ = prof["login"].(string)
		}
		avatar, _ = prof["avatar_url"].(string)
		// SECURITY: ignore the (possibly unverified) profile email entirely; only
		// use the primary, verified email from the emails API.
		email = githubPrimaryEmail(accessToken)
	}
	return id, email, name, avatar, nil
}

// googleEmailVerified handles email_verified arriving as a bool or a string.
func googleEmailVerified(prof map[string]any) bool {
	switch v := prof["email_verified"].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
}

func getJSON(u, accessToken string) (map[string]any, error) {
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "engram-console")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func githubPrimaryEmail(accessToken string) string {
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/user/emails", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "engram-console")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if json.Unmarshal(body, &emails) != nil {
		return ""
	}
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email
		}
	}
	return ""
}
