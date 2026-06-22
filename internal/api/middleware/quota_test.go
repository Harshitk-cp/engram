package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
)

// fakeBillingStore is an in-memory domain.BillingStore for middleware tests.
type fakeBillingStore struct {
	mu         sync.Mutex
	plan       domain.Plan
	memories   int64
	recalls    int64
	agentCount int
	getErr     error
}

func (f *fakeBillingStore) GetBilling(_ context.Context, tid uuid.UUID) (*domain.Billing, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return &domain.Billing{TenantID: tid, Plan: f.plan, SubscriptionStatus: "active"}, nil
}
func (f *fakeBillingStore) SetRazorpayCustomer(context.Context, uuid.UUID, string) error { return nil }
func (f *fakeBillingStore) SetSubscription(context.Context, uuid.UUID, domain.Plan, string, string) error {
	return nil
}
func (f *fakeBillingStore) SetPlan(context.Context, uuid.UUID, domain.Plan, string) error { return nil }
func (f *fakeBillingStore) CurrentUsage(_ context.Context, _ uuid.UUID) (*domain.Usage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &domain.Usage{MemoriesWritten: f.memories, Recalls: f.recalls}, nil
}
func (f *fakeBillingStore) IncrementMemories(_ context.Context, _ uuid.UUID, n int64) error {
	f.mu.Lock()
	f.memories += n
	f.mu.Unlock()
	return nil
}
func (f *fakeBillingStore) IncrementRecalls(_ context.Context, _ uuid.UUID, n int64) error {
	f.mu.Lock()
	f.recalls += n
	f.mu.Unlock()
	return nil
}
func (f *fakeBillingStore) CountAgents(context.Context, uuid.UUID) (int, error) {
	return f.agentCount, nil
}

// withTenant injects an auth context carrying the given tenant, matching what
// SessionOrAPIKey sets up in production.
func withTenant(r *http.Request) *http.Request {
	auth := &domain.APIKeyAuth{Tenant: &domain.Tenant{ID: uuid.New()}, Scopes: []string{"admin", "read", "write"}}
	return r.WithContext(context.WithValue(r.Context(), authContextKey, auth))
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusCreated) })
}

func TestEnforceMemoryQuota_UnderLimit(t *testing.T) {
	store := &fakeBillingStore{plan: domain.PlanDeveloper, memories: 10}
	h := EnforceMemoryQuota(store, true)(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withTenant(httptest.NewRequest(http.MethodPost, "/v1/memories", nil)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}

func TestEnforceMemoryQuota_AtLimitBlocks(t *testing.T) {
	limit := domain.LimitsFor(domain.PlanDeveloper).MaxMemoriesPerMonth
	store := &fakeBillingStore{plan: domain.PlanDeveloper, memories: limit}
	h := EnforceMemoryQuota(store, true)(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withTenant(httptest.NewRequest(http.MethodPost, "/v1/memories", nil)))

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402 at limit, got %d", rec.Code)
	}
}

func TestEnforceMemoryQuota_DisabledIsPassthrough(t *testing.T) {
	limit := domain.LimitsFor(domain.PlanFree).MaxMemoriesPerMonth
	store := &fakeBillingStore{plan: domain.PlanFree, memories: limit + 100}
	// enabled=false → no enforcement even though way over the free cap.
	h := EnforceMemoryQuota(store, false)(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withTenant(httptest.NewRequest(http.MethodPost, "/v1/memories", nil)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("disabled middleware should pass through, got %d", rec.Code)
	}
}

func TestEnforceMemoryQuota_EnterpriseUnlimited(t *testing.T) {
	store := &fakeBillingStore{plan: domain.PlanEnterprise, memories: 99_000_000}
	h := EnforceMemoryQuota(store, true)(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withTenant(httptest.NewRequest(http.MethodPost, "/v1/memories", nil)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("enterprise should be unlimited, got %d", rec.Code)
	}
}

func TestEnforceAgentQuota_AtLimitBlocks(t *testing.T) {
	max := domain.LimitsFor(domain.PlanDeveloper).MaxAgents
	store := &fakeBillingStore{plan: domain.PlanDeveloper, agentCount: max}
	h := EnforceAgentQuota(store, true)(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withTenant(httptest.NewRequest(http.MethodPost, "/v1/agents", nil)))

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402 at agent limit, got %d", rec.Code)
	}
}

func TestEnforceMemoryQuota_FailsOpenOnStoreError(t *testing.T) {
	store := &fakeBillingStore{plan: domain.PlanFree, getErr: context.DeadlineExceeded}
	h := EnforceMemoryQuota(store, true)(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withTenant(httptest.NewRequest(http.MethodPost, "/v1/memories", nil)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("should fail open on lookup error, got %d", rec.Code)
	}
}
