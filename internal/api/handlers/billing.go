package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/Harshitk-cp/engram/internal/api/middleware"
	"github.com/Harshitk-cp/engram/internal/billing"
	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// BillingHandler serves the managed-cloud billing surface: current plan + usage,
// Razorpay subscription creation, client-side payment verification, cancellation,
// and the Razorpay webhook.
type BillingHandler struct {
	store   domain.BillingStore
	rzp     *billing.Client
	baseURL string
	logger  *zap.Logger
}

func NewBillingHandler(store domain.BillingStore, rzp *billing.Client, baseURL string, logger *zap.Logger) *BillingHandler {
	return &BillingHandler{store: store, rzp: rzp, baseURL: baseURL, logger: logger}
}

type billingStateResponse struct {
	Plan       domain.Plan       `json:"plan"`
	Status     string            `json:"subscription_status"`
	Limits     domain.PlanLimits `json:"limits"`
	Usage      *domain.Usage     `json:"usage"`
	AgentCount int               `json:"agent_count"`
	Enabled    bool              `json:"billing_enabled"`
	Plans      []planOption      `json:"plans"`
}

type planOption struct {
	Plan        domain.Plan       `json:"plan"`
	Limits      domain.PlanLimits `json:"limits"`
	Purchasable bool              `json:"purchasable"`
}

// Get returns the org's plan, limits, and current-period usage for the console.
func (h *BillingHandler) Get(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	b, err := h.store.GetBilling(r.Context(), tenant.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load billing")
		return
	}
	usage, err := h.store.CurrentUsage(r.Context(), tenant.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load usage")
		return
	}
	agentCount, _ := h.store.CountAgents(r.Context(), tenant.ID)

	resp := billingStateResponse{
		Plan:       b.Plan,
		Status:     b.SubscriptionStatus,
		Limits:     domain.LimitsFor(b.Plan),
		Usage:      usage,
		AgentCount: agentCount,
		Enabled:    h.rzp.Enabled(),
	}
	for _, p := range []domain.Plan{domain.PlanDeveloper, domain.PlanTeam, domain.PlanGrowth} {
		resp.Plans = append(resp.Plans, planOption{
			Plan:        p,
			Limits:      domain.LimitsFor(p),
			Purchasable: h.rzp.HasPlan(p),
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

type checkoutRequest struct {
	Plan string `json:"plan"`
}

type checkoutResponse struct {
	SubscriptionID string `json:"subscription_id"`
	KeyID          string `json:"key_id"`
}

// Checkout creates a Razorpay subscription for a self-serve plan upgrade and
// returns the subscription id + public key the browser needs to open the Checkout
// modal. The subscription only becomes active once payment succeeds (verified via
// /verify and the subscription.* webhooks).
func (h *BillingHandler) Checkout(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !h.rzp.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "billing not configured")
		return
	}
	var req checkoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	plan := domain.Plan(req.Plan)
	if !plan.SelfServe() {
		writeError(w, http.StatusBadRequest, "plan is not self-serve")
		return
	}

	subID, err := h.rzp.CreateSubscription(r.Context(), billing.SubscribeParams{
		Plan:     plan,
		TenantID: tenant.ID.String(),
	})
	if err != nil {
		if err == billing.ErrNoPlan {
			writeError(w, http.StatusBadRequest, "plan not available for purchase")
			return
		}
		h.logger.Error("razorpay create subscription failed", zap.Error(err))
		writeError(w, http.StatusBadGateway, "failed to start checkout")
		return
	}
	writeJSON(w, http.StatusOK, checkoutResponse{SubscriptionID: subID, KeyID: h.rzp.KeyID()})
}

type verifyRequest struct {
	PaymentID      string `json:"razorpay_payment_id"`
	SubscriptionID string `json:"razorpay_subscription_id"`
	Signature      string `json:"razorpay_signature"`
}

// Verify validates the signature returned by the Checkout modal's success handler.
// On success it optimistically marks the org active so the console reflects the
// upgrade immediately; the subscription.* webhook remains the source of truth for
// the plan and ongoing status.
func (h *BillingHandler) Verify(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !h.rzp.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "billing not configured")
		return
	}
	var req verifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !h.rzp.VerifyPaymentSignature(req.PaymentID, req.SubscriptionID, req.Signature) {
		writeError(w, http.StatusBadRequest, "payment signature verification failed")
		return
	}
	// Record the subscription id and flip to active; the precise plan is reconciled
	// from the webhook. Leave the plan unchanged here to avoid trusting client input.
	b, err := h.store.GetBilling(r.Context(), tenant.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load billing")
		return
	}
	if err := h.store.SetSubscription(r.Context(), tenant.ID, b.Plan, "active", req.SubscriptionID); err != nil {
		h.logger.Error("apply verified subscription failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to record subscription")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Cancel cancels the org's Razorpay subscription at the end of the current billing
// cycle. Razorpay has no hosted customer portal, so this replaces it. The downgrade
// to the free tier arrives via the subscription.cancelled webhook.
func (h *BillingHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !h.rzp.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "billing not configured")
		return
	}
	b, err := h.store.GetBilling(r.Context(), tenant.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load billing")
		return
	}
	if b.RazorpaySubscriptionID == "" {
		writeError(w, http.StatusBadRequest, "no active subscription to cancel")
		return
	}
	if err := h.rzp.CancelSubscription(r.Context(), b.RazorpaySubscriptionID, true); err != nil {
		h.logger.Error("razorpay cancel failed", zap.Error(err))
		writeError(w, http.StatusBadGateway, "failed to cancel subscription")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Webhook receives Razorpay events (unauthenticated; verified via signature) and
// reconciles the org's plan. Always returns 200 on a well-formed, verified event so
// Razorpay stops retrying. Orgs are resolved from the subscription's notes[tenant_id].
func (h *BillingHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	event, err := h.rzp.VerifyAndParse(payload, r.Header.Get("X-Razorpay-Signature"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid webhook: "+err.Error())
		return
	}

	switch event.Type {
	case "subscription.activated", "subscription.charged", "subscription.updated":
		h.applySubscription(r, event, false)
	case "subscription.pending", "subscription.halted":
		h.applyStatus(r, event)
	case "subscription.cancelled", "subscription.completed":
		h.applySubscription(r, event, true)
	}

	w.WriteHeader(http.StatusOK)
}

// tenantFromEvent resolves the org owning a webhook's subscription via the
// notes[tenant_id] we set at checkout. Returns uuid.Nil when absent/malformed.
func tenantFromEvent(event *billing.Event) (uuid.UUID, bool) {
	if event.TenantID == "" {
		return uuid.Nil, false
	}
	tid, err := uuid.Parse(event.TenantID)
	if err != nil {
		return uuid.Nil, false
	}
	return tid, true
}

// applySubscription reconciles an org's plan from a subscription event. On
// cancellation/completion (canceled == true) the org drops to the free tier.
func (h *BillingHandler) applySubscription(r *http.Request, event *billing.Event, canceled bool) {
	tid, ok := tenantFromEvent(event)
	if !ok {
		h.logger.Warn("webhook with no tenant_id in notes", zap.String("subscription", event.SubscriptionID))
		return
	}
	if canceled {
		if err := h.store.SetPlan(r.Context(), tid, domain.PlanFree, "canceled"); err != nil {
			h.logger.Error("downgrade to free failed", zap.Error(err))
		}
		return
	}

	b, err := h.store.GetBilling(r.Context(), tid)
	if err != nil {
		h.logger.Warn("webhook for unknown tenant", zap.String("tenant", tid.String()))
		return
	}
	plan := b.Plan
	if p, ok := h.rzp.PlanForPlanID(event.PlanID); ok {
		plan = p // unknown plan id — leave plan unchanged
	}
	status := event.Status
	if status == "" {
		status = "active"
	}
	if err := h.store.SetSubscription(r.Context(), tid, plan, status, event.SubscriptionID); err != nil {
		h.logger.Error("apply subscription failed", zap.Error(err))
	}
	if event.CustomerID != "" {
		if err := h.store.SetRazorpayCustomer(r.Context(), tid, event.CustomerID); err != nil {
			h.logger.Error("set razorpay customer failed", zap.Error(err))
		}
	}
}

// applyStatus flags a payment-trouble state (pending/halted) without changing the
// plan, so quota stays enforced while Razorpay retries collection.
func (h *BillingHandler) applyStatus(r *http.Request, event *billing.Event) {
	tid, ok := tenantFromEvent(event)
	if !ok {
		return
	}
	b, err := h.store.GetBilling(r.Context(), tid)
	if err != nil {
		return
	}
	status := event.Status
	if status == "" {
		status = "past_due"
	}
	if err := h.store.SetSubscription(r.Context(), tid, b.Plan, status, event.SubscriptionID); err != nil {
		h.logger.Error("apply subscription status failed", zap.Error(err))
	}
}
