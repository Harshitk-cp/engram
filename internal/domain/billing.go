package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Plan is a managed-cloud subscription tier. The data plane stays plan-agnostic;
// only the quota middleware and billing handler read it. Pricing tiers mirror the
// roadmap: Developer $29 / Team $149 / Growth $499 (see engram_docs/plan.md P2-2).
type Plan string

const (
	PlanFree       Plan = "free"
	PlanDeveloper  Plan = "developer"
	PlanTeam       Plan = "team"
	PlanGrowth     Plan = "growth"
	PlanEnterprise Plan = "enterprise"
)

// PlanLimits are the hard caps enforced per org per billing period. A limit of
// -1 means unlimited. PriceUSD is the monthly list price, used for console display
// only — the authoritative price lives in Stripe.
type PlanLimits struct {
	MaxAgents           int   `json:"max_agents"`
	MaxMemoriesPerMonth int64 `json:"max_memories_per_month"`
	PriceUSD            int   `json:"price_usd"`
}

// Unlimited is the sentinel used by PlanLimits for "no cap".
const Unlimited = -1

var planLimits = map[Plan]PlanLimits{
	PlanFree:       {MaxAgents: 1, MaxMemoriesPerMonth: 1_000, PriceUSD: 0},
	PlanDeveloper:  {MaxAgents: 5, MaxMemoriesPerMonth: 50_000, PriceUSD: 29},
	PlanTeam:       {MaxAgents: 25, MaxMemoriesPerMonth: 500_000, PriceUSD: 149},
	PlanGrowth:     {MaxAgents: 100, MaxMemoriesPerMonth: 5_000_000, PriceUSD: 499},
	PlanEnterprise: {MaxAgents: Unlimited, MaxMemoriesPerMonth: Unlimited, PriceUSD: 0},
}

// LimitsFor returns the caps for a plan, falling back to the free tier for any
// unknown value so a malformed plan column never grants unlimited usage.
func LimitsFor(p Plan) PlanLimits {
	if l, ok := planLimits[p]; ok {
		return l
	}
	return planLimits[PlanFree]
}

// Valid reports whether p is a known plan.
func (p Plan) Valid() bool {
	_, ok := planLimits[p]
	return ok
}

// SelfServe reports whether a plan can be purchased via Stripe Checkout. Free is
// the default (no purchase) and Enterprise is sales-assisted (set manually).
func (p Plan) SelfServe() bool {
	return p == PlanDeveloper || p == PlanTeam || p == PlanGrowth
}

// Billing is an org's subscription state.
type Billing struct {
	TenantID             uuid.UUID `json:"tenant_id"`
	Plan                 Plan      `json:"plan"`
	SubscriptionStatus   string    `json:"subscription_status"`
	StripeCustomerID     string    `json:"-"`
	StripeSubscriptionID string    `json:"-"`
}

// Usage is an org's metered consumption for a single billing period.
type Usage struct {
	PeriodMonth     time.Time `json:"period_month"`
	MemoriesWritten int64     `json:"memories_written"`
	Recalls         int64     `json:"recalls"`
}

// BillingStore persists per-org plan state and monthly usage counters.
type BillingStore interface {
	// GetBilling returns the org's current plan + Stripe references.
	GetBilling(ctx context.Context, tenantID uuid.UUID) (*Billing, error)
	// GetByStripeCustomer resolves the org owning a Stripe customer (webhook path).
	GetByStripeCustomer(ctx context.Context, customerID string) (*Billing, error)
	// SetStripeCustomer records the Stripe customer id created at checkout.
	SetStripeCustomer(ctx context.Context, tenantID uuid.UUID, customerID string) error
	// SetSubscription updates plan + status + subscription id from a Stripe event.
	SetSubscription(ctx context.Context, tenantID uuid.UUID, plan Plan, status, subscriptionID string) error
	// SetPlan sets the plan directly without Stripe (manual/enterprise contracts).
	SetPlan(ctx context.Context, tenantID uuid.UUID, plan Plan, status string) error

	// CurrentUsage returns usage for the current UTC month (zeroed if none yet).
	CurrentUsage(ctx context.Context, tenantID uuid.UUID) (*Usage, error)
	// IncrementMemories atomically adds to the current month's memory write count.
	IncrementMemories(ctx context.Context, tenantID uuid.UUID, n int64) error
	// IncrementRecalls atomically adds to the current month's recall count.
	IncrementRecalls(ctx context.Context, tenantID uuid.UUID, n int64) error
	// CountAgents returns the org's live (non-archived) agent count.
	CountAgents(ctx context.Context, tenantID uuid.UUID) (int, error)
}
