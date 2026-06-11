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
// Stripe Checkout, the customer Billing Portal, and the Stripe webhook.
type BillingHandler struct {
	store   domain.BillingStore
	stripe  *billing.Client
	baseURL string
	logger  *zap.Logger
}

func NewBillingHandler(store domain.BillingStore, stripe *billing.Client, baseURL string, logger *zap.Logger) *BillingHandler {
	return &BillingHandler{store: store, stripe: stripe, baseURL: baseURL, logger: logger}
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
		Enabled:    h.stripe.Enabled(),
	}
	for _, p := range []domain.Plan{domain.PlanDeveloper, domain.PlanTeam, domain.PlanGrowth} {
		resp.Plans = append(resp.Plans, planOption{
			Plan:        p,
			Limits:      domain.LimitsFor(p),
			Purchasable: h.stripe.HasPrice(p),
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

type checkoutRequest struct {
	Plan string `json:"plan"`
}

// Checkout starts a Stripe Checkout session for a self-serve plan upgrade.
func (h *BillingHandler) Checkout(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !h.stripe.Enabled() {
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

	b, err := h.store.GetBilling(r.Context(), tenant.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load billing")
		return
	}

	url, err := h.stripe.CreateCheckoutSession(r.Context(), billing.CheckoutParams{
		Plan:       plan,
		TenantID:   tenant.ID.String(),
		CustomerID: b.StripeCustomerID,
		SuccessURL: h.baseURL + "/settings?billing=success",
		CancelURL:  h.baseURL + "/settings?billing=cancel",
	})
	if err != nil {
		if err == billing.ErrNoPrice {
			writeError(w, http.StatusBadRequest, "plan not available for purchase")
			return
		}
		h.logger.Error("stripe checkout failed", zap.Error(err))
		writeError(w, http.StatusBadGateway, "failed to create checkout session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

// Portal opens the Stripe Billing Portal so the customer can change or cancel.
func (h *BillingHandler) Portal(w http.ResponseWriter, r *http.Request) {
	tenant := middleware.TenantFromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !h.stripe.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "billing not configured")
		return
	}
	b, err := h.store.GetBilling(r.Context(), tenant.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load billing")
		return
	}
	if b.StripeCustomerID == "" {
		writeError(w, http.StatusBadRequest, "no active subscription to manage")
		return
	}
	url, err := h.stripe.CreatePortalSession(r.Context(), b.StripeCustomerID, h.baseURL+"/settings")
	if err != nil {
		h.logger.Error("stripe portal failed", zap.Error(err))
		writeError(w, http.StatusBadGateway, "failed to open billing portal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

// Webhook receives Stripe events (unauthenticated; verified via signature) and
// reconciles the org's plan. Always returns 200 on a well-formed, verified event
// so Stripe stops retrying.
func (h *BillingHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	event, err := h.stripe.VerifyAndParse(payload, r.Header.Get("Stripe-Signature"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid webhook: "+err.Error())
		return
	}

	switch event.Type {
	case "checkout.session.completed":
		// First successful purchase: bind the Stripe customer to the org. The
		// subscription's plan/status arrives via the subscription.* events.
		if event.ClientReferenceID != "" && event.CustomerID != "" {
			if tid, perr := uuid.Parse(event.ClientReferenceID); perr == nil {
				if serr := h.store.SetStripeCustomer(r.Context(), tid, event.CustomerID); serr != nil {
					h.logger.Error("set stripe customer failed", zap.Error(serr))
				}
			}
		}
	case "customer.subscription.created", "customer.subscription.updated":
		h.applySubscription(r, event, false)
	case "customer.subscription.deleted":
		h.applySubscription(r, event, true)
	}

	w.WriteHeader(http.StatusOK)
}

// applySubscription resolves the org by Stripe customer and updates its plan. On
// cancellation (canceled == true) the org drops to the free tier.
func (h *BillingHandler) applySubscription(r *http.Request, event *billing.Event, canceled bool) {
	if event.CustomerID == "" {
		return
	}
	b, err := h.store.GetByStripeCustomer(r.Context(), event.CustomerID)
	if err != nil {
		h.logger.Warn("webhook for unknown stripe customer", zap.String("customer", event.CustomerID))
		return
	}

	plan := domain.PlanFree
	status := "canceled"
	if !canceled {
		if p, ok := h.stripe.PlanForPrice(event.PriceID); ok {
			plan = p
		} else {
			plan = b.Plan // unknown price — leave plan unchanged
		}
		status = event.Status
		if status == "" {
			status = "active"
		}
	}
	if err := h.store.SetSubscription(r.Context(), b.TenantID, plan, status, event.SubscriptionID); err != nil {
		h.logger.Error("apply subscription failed", zap.Error(err))
	}
}
