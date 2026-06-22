// Package billing wraps the slice of the Razorpay API the managed cloud needs:
// creating subscriptions, verifying the client-side checkout payment signature,
// cancelling subscriptions, and verifying inbound webhook signatures. It talks to
// Razorpay over plain HTTP so the project takes on no third-party SDK dependency,
// and it degrades to a clean no-op when no API keys are configured (self-hosted / OSS).
package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
)

// ErrNotConfigured is returned by subscription/cancel calls when Razorpay is disabled.
var ErrNotConfigured = errors.New("billing not configured")

// ErrNoPlan is returned when the requested plan has no configured Razorpay plan id.
var ErrNoPlan = errors.New("no plan configured for tier")

const defaultBaseURL = "https://api.razorpay.com"

// subscriptionTotalCount is the number of billing cycles a self-serve subscription
// runs for before Razorpay auto-completes it. Razorpay requires total_count up front,
// so we set a long horizon (120 monthly cycles ≈ 10 years) to approximate an
// "until cancelled" subscription. Tune if a shorter commitment is desired.
const subscriptionTotalCount = 120

// Client is a minimal Razorpay REST client.
type Client struct {
	keyID         string
	keySecret     string
	webhookSecret string
	planIDs       map[string]string // plan tier name -> Razorpay plan id
	httpClient    *http.Client
	baseURL       string
}

// New builds a Razorpay client. planIDs is keyed by plan tier name
// (developer/team/growth). When keyID or keySecret is empty the client reports
// Enabled() == false and all calls no-op.
func New(keyID, keySecret, webhookSecret string, planIDs map[string]string) *Client {
	return &Client{
		keyID:         keyID,
		keySecret:     keySecret,
		webhookSecret: webhookSecret,
		planIDs:       planIDs,
		httpClient:    &http.Client{Timeout: 15 * time.Second},
		baseURL:       defaultBaseURL,
	}
}

// Enabled reports whether Razorpay API credentials are configured.
func (c *Client) Enabled() bool { return c.keyID != "" && c.keySecret != "" }

// KeyID returns the public Razorpay key id, safe to hand to the browser to open
// the Checkout modal.
func (c *Client) KeyID() string { return c.keyID }

// HasPlan reports whether a self-serve Razorpay plan is configured for the tier.
func (c *Client) HasPlan(plan domain.Plan) bool {
	_, ok := c.planIDs[string(plan)]
	return ok
}

// SubscribeParams configures a subscription created for the embedded Checkout modal.
type SubscribeParams struct {
	Plan          domain.Plan
	TenantID      string // stored in notes[tenant_id], echoed back on webhooks
	CustomerEmail string // pre-fills the customer's email on Razorpay notifications
}

// CreateSubscription creates a Razorpay subscription in the "created" state and
// returns its id. The browser then opens the Checkout modal with this id + the
// public key to collect payment.
func (c *Client) CreateSubscription(ctx context.Context, p SubscribeParams) (string, error) {
	if !c.Enabled() {
		return "", ErrNotConfigured
	}
	planID, ok := c.planIDs[string(p.Plan)]
	if !ok {
		return "", ErrNoPlan
	}

	form := url.Values{}
	form.Set("plan_id", planID)
	form.Set("total_count", fmt.Sprintf("%d", subscriptionTotalCount))
	form.Set("customer_notify", "1")
	form.Set("notes[tenant_id]", p.TenantID)
	if p.CustomerEmail != "" {
		form.Set("notify_info[notify_email]", p.CustomerEmail)
	}

	var out struct {
		ID string `json:"id"`
	}
	if err := c.post(ctx, "/v1/subscriptions", form, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// CancelSubscription cancels a Razorpay subscription. When atCycleEnd is true the
// subscription stays active until the end of the current billing cycle (the org
// keeps the access it paid for); the downgrade to free arrives via the
// subscription.cancelled webhook.
func (c *Client) CancelSubscription(ctx context.Context, subscriptionID string, atCycleEnd bool) error {
	if !c.Enabled() {
		return ErrNotConfigured
	}
	if subscriptionID == "" {
		return errors.New("no razorpay subscription to cancel")
	}
	form := url.Values{}
	if atCycleEnd {
		form.Set("cancel_at_cycle_end", "1")
	} else {
		form.Set("cancel_at_cycle_end", "0")
	}
	return c.post(ctx, "/v1/subscriptions/"+url.PathEscape(subscriptionID)+"/cancel", form, nil)
}

func (c *Client) post(ctx context.Context, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.keyID, c.keySecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("razorpay %s: status %d: %s", path, resp.StatusCode, string(body))
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// PlanForPlanID reverse-maps a Razorpay plan id to a plan tier. Returns ("", false)
// when the plan id is unknown.
func (c *Client) PlanForPlanID(planID string) (domain.Plan, bool) {
	for name, id := range c.planIDs {
		if id == planID {
			return domain.Plan(name), true
		}
	}
	return "", false
}

// ---- Client-side payment verification ----

// VerifyPaymentSignature checks the signature returned by the Checkout modal's
// success handler. For subscriptions Razorpay signs "<payment_id>|<subscription_id>"
// with the key secret (note the order differs from one-off orders). Returns true
// when the signature is valid.
func (c *Client) VerifyPaymentSignature(paymentID, subscriptionID, signature string) bool {
	if c.keySecret == "" || paymentID == "" || subscriptionID == "" || signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(c.keySecret))
	mac.Write([]byte(paymentID + "|" + subscriptionID))
	expected := mac.Sum(nil)
	got, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	return hmac.Equal(got, expected)
}

// ---- Webhook handling ----

// Event is the decoded slice of a Razorpay webhook we act on, flattened from the
// subscription entity carried by subscription.* events.
type Event struct {
	Type           string // e.g. subscription.activated, subscription.charged
	TenantID       string // from the subscription's notes[tenant_id]
	CustomerID     string
	SubscriptionID string
	Status         string // subscription status (active, pending, halted, cancelled, ...)
	PlanID         string
}

// VerifyAndParse checks the X-Razorpay-Signature header against the configured
// webhook secret, then decodes the event. When no webhook secret is configured the
// signature check is skipped (test/dev convenience), but the payload is still parsed.
func (c *Client) VerifyAndParse(payload []byte, sigHeader string) (*Event, error) {
	if c.webhookSecret != "" {
		if err := verifyWebhookSignature(payload, sigHeader, c.webhookSecret); err != nil {
			return nil, err
		}
	}
	return parseEvent(payload)
}

func parseEvent(payload []byte) (*Event, error) {
	var raw struct {
		Event   string `json:"event"`
		Payload struct {
			Subscription struct {
				Entity struct {
					ID         string            `json:"id"`
					PlanID     string            `json:"plan_id"`
					CustomerID string            `json:"customer_id"`
					Status     string            `json:"status"`
					Notes      map[string]string `json:"notes"`
				} `json:"entity"`
			} `json:"subscription"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, err
	}
	ent := raw.Payload.Subscription.Entity
	e := &Event{
		Type:           raw.Event,
		CustomerID:     ent.CustomerID,
		SubscriptionID: ent.ID,
		Status:         ent.Status,
		PlanID:         ent.PlanID,
	}
	if ent.Notes != nil {
		e.TenantID = ent.Notes["tenant_id"]
	}
	return e, nil
}

// verifyWebhookSignature implements Razorpay's webhook scheme: the X-Razorpay-Signature
// header is the hex-encoded HMAC-SHA256 of the raw request body keyed by the webhook
// secret.
func verifyWebhookSignature(payload []byte, header, secret string) error {
	if header == "" {
		return errors.New("missing x-razorpay-signature header")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := mac.Sum(nil)
	got, err := hex.DecodeString(header)
	if err != nil {
		return errors.New("invalid x-razorpay-signature header")
	}
	if !hmac.Equal(got, expected) {
		return errors.New("razorpay webhook signature mismatch")
	}
	return nil
}
