package transports

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
)

// maxBodyBytes caps JSON-RPC request bodies on the network transports,
// matching the stdio transport's 4 MB line limit.
const maxBodyBytes = 4 << 20

// Guard wraps an MCP network transport with the protections a privileged
// local server needs. The transport holds the operator's ENGRAM_API_KEY, so
// reaching it at all means full memory read/write/delete.
func Guard(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" {
			const prefix = "Bearer "
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, prefix) ||
				subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(auth, prefix)), []byte(token)) != 1 {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		if r.Header.Get("Origin") != "" {
			http.Error(w, "browser origins are not allowed; set ENGRAM_MCP_TOKEN to enable authenticated access", http.StatusForbidden)
			return
		}
		if !isLoopbackHost(r.Host) {
			http.Error(w, "invalid host", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// IsLoopbackAddr reports whether a listen address binds only the loopback
// interface.
func IsLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		return false
	}
	return isLoopbackHost(host)
}

func isLoopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}
