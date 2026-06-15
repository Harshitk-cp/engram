// Package billing wraps the slice of the Stripe API the managed cloud needs:
// creating subscription Checkout sessions, opening the customer Billing Portal,
// and verifying inbound webhook signatures. It talks to Stripe over plain HTTP so
// the project takes on no third-party SDK dependency, and it degrades to a clean
// no-op when no secret key is configured (self-hosted / OSS).
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
	"strconv"
	"strings"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
)

// ErrNotConfigured is returned by checkout/portal calls when Stripe is disabled.
var ErrNotConfigured = errors.New("billing not configured")

// ErrNoPrice is returned when the requested plan has no configured Stripe price.
var ErrNoPrice = errors.New("no price configured for plan")

const defaultBaseURL = "https://api.stripe.com"

// webhookTolerance bounds the age of an accepted webhook to mitigate replay.
const webhookTolerance = 5 * time.Minute

// Client is a minimal Stripe REST client.
type Client struct {
	secretKey     string
	webhookSecret string
	priceIDs      map[string]string // plan name -> Stripe price id
	httpClient    *http.Client
	baseURL       string
}

// New builds a Stripe client. priceIDs is keyed by plan name (developer/team/growth).
// When secretKey is empty the client reports Enabled() == false and all calls no-op.
func New(secretKey, webhookSecret string, priceIDs map[string]string) *Client {
	return &Client{
		secretKey:     secretKey,
		webhookSecret: webhookSecret,
		priceIDs:      priceIDs,
		httpClient:    &http.Client{Timeout: 15 * time.Second},
		baseURL:       defaultBaseURL,
	}
}

// Enabled reports whether a Stripe secret key is configured.
func (c *Client) Enabled() bool { return c.secretKey != "" }

// HasPrice reports whether a self-serve price is configured for the plan.
func (c *Client) HasPrice(plan domain.Plan) bool {
	_, ok := c.priceIDs[string(plan)]
	return ok
}

// CheckoutParams configures a subscription Checkout session.
type CheckoutParams struct {
	Plan          domain.Plan
	TenantID      string // becomes client_reference_id, echoed back on the event
	CustomerID    string // reuse an existing Stripe customer if known
	CustomerEmail string // pre-fills checkout when no customer exists yet
	SuccessURL    string
	CancelURL     string
}

// CreateCheckoutSession creates a Stripe Checkout session and returns its URL.
func (c *Client) CreateCheckoutSession(ctx context.Context, p CheckoutParams) (string, error) {
	if !c.Enabled() {
		return "", ErrNotConfigured
	}
	priceID, ok := c.priceIDs[string(p.Plan)]
	if !ok {
		return "", ErrNoPrice
	}

	form := url.Values{}
	form.Set("mode", "subscription")
	form.Set("line_items[0][price]", priceID)
	form.Set("line_items[0][quantity]", "1")
	form.Set("success_url", p.SuccessURL)
	form.Set("cancel_url", p.CancelURL)
	form.Set("client_reference_id", p.TenantID)
	form.Set("allow_promotion_codes", "true")
	if p.CustomerID != "" {
		form.Set("customer", p.CustomerID)
	} else if p.CustomerEmail != "" {
		form.Set("customer_email", p.CustomerEmail)
	}

	var out struct {
		URL string `json:"url"`
	}
	if err := c.post(ctx, "/v1/checkout/sessions", form, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

// CreatePortalSession opens the Stripe customer Billing Portal for self-service
// plan changes and cancellation.
func (c *Client) CreatePortalSession(ctx context.Context, customerID, returnURL string) (string, error) {
	if !c.Enabled() {
		return "", ErrNotConfigured
	}
	if customerID == "" {
		return "", errors.New("no stripe customer for org")
	}
	form := url.Values{}
	form.Set("customer", customerID)
	form.Set("return_url", returnURL)

	var out struct {
		URL string `json:"url"`
	}
	if err := c.post(ctx, "/v1/billing_portal/sessions", form, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

func (c *Client) post(ctx context.Context, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.secretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("stripe %s: status %d: %s", path, resp.StatusCode, string(body))
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// ---- Webhook handling ----

// Event is the decoded slice of a Stripe webhook we act on.
type Event struct {
	Type string
	// Object fields, flattened from data.object across the event types we handle.
	ClientReferenceID string // checkout.session.completed
	CustomerID        string
	SubscriptionID    string
	Status            string // subscription status (active, past_due, canceled, ...)
	PriceID           string // first line item price, maps back to a plan
}

// VerifyAndParse checks the Stripe-Signature header against the configured webhook
// secret, then decodes the event. When no webhook secret is configured the
// signature check is skipped (test/dev convenience), but the payload is still parsed.
func (c *Client) VerifyAndParse(payload []byte, sigHeader string) (*Event, error) {
	if c.webhookSecret != "" {
		if err := verifySignature(payload, sigHeader, c.webhookSecret); err != nil {
			return nil, err
		}
	}
	return parseEvent(payload)
}

func parseEvent(payload []byte) (*Event, error) {
	// data.object has a literal dot in the JSON key; unmarshal via a shaped struct.
	var raw struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, err
	}
	var dataWrap struct {
		Object struct {
			ClientReferenceID string `json:"client_reference_id"`
			Customer          string `json:"customer"`
			Subscription      string `json:"subscription"`
			Status            string `json:"status"`
			Items             struct {
				Data []struct {
					Price struct {
						ID string `json:"id"`
					} `json:"price"`
				} `json:"data"`
			} `json:"items"`
		} `json:"object"`
	}
	if err := json.Unmarshal(raw.Data, &dataWrap); err != nil {
		return nil, err
	}
	e := &Event{
		Type:              raw.Type,
		ClientReferenceID: dataWrap.Object.ClientReferenceID,
		CustomerID:        dataWrap.Object.Customer,
		SubscriptionID:    dataWrap.Object.Subscription,
		Status:            dataWrap.Object.Status,
	}
	if len(dataWrap.Object.Items.Data) > 0 {
		e.PriceID = dataWrap.Object.Items.Data[0].Price.ID
	}
	return e, nil
}

// PlanForPrice reverse-maps a Stripe price id to a plan. Returns ("", false) when
// the price is unknown.
func (c *Client) PlanForPrice(priceID string) (domain.Plan, bool) {
	for name, id := range c.priceIDs {
		if id == priceID {
			return domain.Plan(name), true
		}
	}
	return "", false
}

// verifySignature implements Stripe's signed-payload scheme:
// header "t=<ts>,v1=<hex hmac>"; signed payload is "<ts>.<body>".
func verifySignature(payload []byte, header, secret string) error {
	var ts string
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			ts = kv[1]
		case "v1":
			sigs = append(sigs, kv[1])
		}
	}
	if ts == "" || len(sigs) == 0 {
		return errors.New("invalid stripe-signature header")
	}
	tsInt, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errors.New("invalid timestamp in stripe-signature")
	}
	if time.Since(time.Unix(tsInt, 0)) > webhookTolerance {
		return errors.New("stripe webhook timestamp outside tolerance")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write(payload)
	expected := mac.Sum(nil)

	for _, s := range sigs {
		got, err := hex.DecodeString(s)
		if err != nil {
			continue
		}
		if hmac.Equal(got, expected) {
			return nil
		}
	}
	return errors.New("stripe webhook signature mismatch")
}
