package middleware

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Harshitk-cp/engram/internal/domain"
)

// quotaResponseWriter captures the status code so post-success metering only
// counts writes that actually succeeded.
type quotaResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *quotaResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *quotaResponseWriter) ok() bool {
	// Default to 200 when the handler never explicitly set a status.
	if w.status == 0 {
		return true
	}
	return w.status >= 200 && w.status < 300
}

// writeQuotaError emits a 402 with the structured upgrade hint the console reads.
func writeQuotaError(w http.ResponseWriter, resource string, limit, used int64) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusPaymentRequired)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":    "plan limit reached for " + resource + " — upgrade your plan to continue",
		"resource": resource,
		"limit":    limit,
		"used":     used,
		"code":     "quota_exceeded",
	})
}

// EnforceMemoryQuota blocks memory writes once the org hits its monthly cap and,
// on a successful write, increments the counter. When enabled is false (no Stripe
// configured) it is a pass-through, so self-hosted/OSS runs unmetered.
func EnforceMemoryQuota(billing domain.BillingStore, enabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if !enabled || billing == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenant := TenantFromContext(r.Context())
			if tenant == nil {
				next.ServeHTTP(w, r)
				return
			}
			b, err := billing.GetBilling(r.Context(), tenant.ID)
			if err != nil {
				// Fail open on a lookup error rather than block a paying customer.
				next.ServeHTTP(w, r)
				return
			}
			limits := domain.LimitsFor(b.Plan)
			if limits.MaxMemoriesPerMonth != domain.Unlimited {
				usage, err := billing.CurrentUsage(r.Context(), tenant.ID)
				if err == nil && usage.MemoriesWritten >= limits.MaxMemoriesPerMonth {
					writeQuotaError(w, "memories", limits.MaxMemoriesPerMonth, usage.MemoriesWritten)
					return
				}
			}

			qw := &quotaResponseWriter{ResponseWriter: w}
			next.ServeHTTP(qw, r)
			if qw.ok() {
				go func() { _ = billing.IncrementMemories(context.Background(), tenant.ID, 1) }()
			}
		})
	}
}

// EnforceAgentQuota blocks agent creation once the org hits its plan's agent cap.
func EnforceAgentQuota(billing domain.BillingStore, enabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if !enabled || billing == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenant := TenantFromContext(r.Context())
			if tenant == nil {
				next.ServeHTTP(w, r)
				return
			}
			b, err := billing.GetBilling(r.Context(), tenant.ID)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			limits := domain.LimitsFor(b.Plan)
			if limits.MaxAgents != domain.Unlimited {
				count, err := billing.CountAgents(r.Context(), tenant.ID)
				if err == nil && count >= limits.MaxAgents {
					writeQuotaError(w, "agents", int64(limits.MaxAgents), int64(count))
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// MeterRecall counts successful recalls without ever blocking them — recall is on
// the agent's hot path and a soft limit there would break live deployments.
func MeterRecall(billing domain.BillingStore, enabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if !enabled || billing == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenant := TenantFromContext(r.Context())
			if tenant == nil {
				next.ServeHTTP(w, r)
				return
			}
			qw := &quotaResponseWriter{ResponseWriter: w}
			next.ServeHTTP(qw, r)
			if qw.ok() {
				go func() { _ = billing.IncrementRecalls(context.Background(), tenant.ID, 1) }()
			}
		})
	}
}
