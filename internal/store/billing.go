package store

import (
	"context"
	"errors"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// monthStartUTC is the first day of the current UTC month, matching the
// date_trunc('month', ... UTC) used for the usage_counters period key.
func monthStartUTC() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// BillingStore reads/writes per-org plan state (on tenants) and the monthly
// usage_counters rollup. See migration 017_billing.
type BillingStore struct {
	db *pgxpool.Pool
}

func NewBillingStore(db *pgxpool.Pool) *BillingStore {
	return &BillingStore{db: db}
}

func (s *BillingStore) scanBilling(row pgx.Row) (*domain.Billing, error) {
	b := &domain.Billing{}
	var plan string
	var customerID, subscriptionID *string
	if err := row.Scan(&b.TenantID, &plan, &b.SubscriptionStatus, &customerID, &subscriptionID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	b.Plan = domain.Plan(plan)
	if customerID != nil {
		b.RazorpayCustomerID = *customerID
	}
	if subscriptionID != nil {
		b.RazorpaySubscriptionID = *subscriptionID
	}
	return b, nil
}

func (s *BillingStore) GetBilling(ctx context.Context, tenantID uuid.UUID) (*domain.Billing, error) {
	return s.scanBilling(s.db.QueryRow(ctx,
		`SELECT id, plan, subscription_status, razorpay_customer_id, razorpay_subscription_id
		   FROM tenants WHERE id = $1`, tenantID))
}

func (s *BillingStore) SetRazorpayCustomer(ctx context.Context, tenantID uuid.UUID, customerID string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE tenants SET razorpay_customer_id = $2, updated_at = NOW() WHERE id = $1`,
		tenantID, customerID)
	return err
}

func (s *BillingStore) SetSubscription(ctx context.Context, tenantID uuid.UUID, plan domain.Plan, status, subscriptionID string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE tenants
		    SET plan = $2, subscription_status = $3, razorpay_subscription_id = $4,
		        plan_period_start = NOW(), updated_at = NOW()
		  WHERE id = $1`,
		tenantID, string(plan), status, subscriptionID)
	return err
}

func (s *BillingStore) SetPlan(ctx context.Context, tenantID uuid.UUID, plan domain.Plan, status string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE tenants SET plan = $2, subscription_status = $3, updated_at = NOW() WHERE id = $1`,
		tenantID, string(plan), status)
	return err
}

func (s *BillingStore) CurrentUsage(ctx context.Context, tenantID uuid.UUID) (*domain.Usage, error) {
	u := &domain.Usage{}
	err := s.db.QueryRow(ctx,
		`SELECT period_month, memories_written, recalls
		   FROM usage_counters
		  WHERE tenant_id = $1 AND period_month = date_trunc('month', now() AT TIME ZONE 'UTC')::date`,
		tenantID,
	).Scan(&u.PeriodMonth, &u.MemoriesWritten, &u.Recalls)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No activity this period yet — report a zeroed counter.
			return &domain.Usage{PeriodMonth: monthStartUTC()}, nil
		}
		return nil, err
	}
	return u, nil
}

func (s *BillingStore) IncrementMemories(ctx context.Context, tenantID uuid.UUID, n int64) error {
	return s.increment(ctx, tenantID, "memories_written", n)
}

func (s *BillingStore) IncrementRecalls(ctx context.Context, tenantID uuid.UUID, n int64) error {
	return s.increment(ctx, tenantID, "recalls", n)
}

// increment performs an atomic upsert into the current month's row. The column
// name is fixed by the caller (never user input), so interpolation is safe.
func (s *BillingStore) increment(ctx context.Context, tenantID uuid.UUID, col string, n int64) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO usage_counters (tenant_id, period_month, `+col+`)
		 VALUES ($1, date_trunc('month', now() AT TIME ZONE 'UTC')::date, $2)
		 ON CONFLICT (tenant_id, period_month)
		 DO UPDATE SET `+col+` = usage_counters.`+col+` + EXCLUDED.`+col+`, updated_at = NOW()`,
		tenantID, n)
	return err
}

func (s *BillingStore) CountAgents(ctx context.Context, tenantID uuid.UUID) (int, error) {
	var count int
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM agents WHERE tenant_id = $1`, tenantID).Scan(&count)
	return count, err
}
