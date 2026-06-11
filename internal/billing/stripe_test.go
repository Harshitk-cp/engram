package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/Harshitk-cp/engram/internal/domain"
)

func sign(t *testing.T, payload []byte, secret string, ts int64) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d", ts)))
	mac.Write([]byte("."))
	mac.Write(payload)
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

func TestVerifyAndParse_ValidSignature(t *testing.T) {
	secret := "whsec_test"
	c := New("sk_test", secret, map[string]string{"team": "price_team"})
	payload := []byte(`{"type":"customer.subscription.updated","data":{"object":{"customer":"cus_1","status":"active","items":{"data":[{"price":{"id":"price_team"}}]}}}}`)
	header := sign(t, payload, secret, time.Now().Unix())

	ev, err := c.VerifyAndParse(payload, header)
	if err != nil {
		t.Fatalf("expected valid event, got error: %v", err)
	}
	if ev.Type != "customer.subscription.updated" {
		t.Errorf("type = %q", ev.Type)
	}
	if ev.CustomerID != "cus_1" || ev.Status != "active" || ev.PriceID != "price_team" {
		t.Errorf("unexpected event: %+v", ev)
	}
	if plan, ok := c.PlanForPrice(ev.PriceID); !ok || plan != domain.PlanTeam {
		t.Errorf("PlanForPrice = %q, %v", plan, ok)
	}
}

func TestVerifyAndParse_BadSignature(t *testing.T) {
	c := New("sk_test", "whsec_test", nil)
	payload := []byte(`{"type":"x","data":{"object":{}}}`)
	if _, err := c.VerifyAndParse(payload, "t=123,v1=deadbeef"); err == nil {
		t.Fatal("expected signature error")
	}
}

func TestVerifyAndParse_StaleTimestamp(t *testing.T) {
	secret := "whsec_test"
	c := New("sk_test", secret, nil)
	payload := []byte(`{"type":"x","data":{"object":{}}}`)
	old := time.Now().Add(-10 * time.Minute).Unix()
	if _, err := c.VerifyAndParse(payload, sign(t, payload, secret, old)); err == nil {
		t.Fatal("expected stale-timestamp rejection")
	}
}

func TestVerifyAndParse_NoSecretSkipsVerification(t *testing.T) {
	// With no webhook secret (dev/test), the payload still parses without a sig.
	c := New("sk_test", "", nil)
	payload := []byte(`{"type":"checkout.session.completed","data":{"object":{"client_reference_id":"abc","customer":"cus_9"}}}`)
	ev, err := c.VerifyAndParse(payload, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.ClientReferenceID != "abc" || ev.CustomerID != "cus_9" {
		t.Errorf("unexpected event: %+v", ev)
	}
}

func TestClientDisabledWithoutSecret(t *testing.T) {
	c := New("", "", nil)
	if c.Enabled() {
		t.Error("client should be disabled without secret key")
	}
	if _, err := c.CreateCheckoutSession(context.Background(), CheckoutParams{Plan: domain.PlanTeam}); err != ErrNotConfigured {
		t.Errorf("expected ErrNotConfigured, got %v", err)
	}
}
