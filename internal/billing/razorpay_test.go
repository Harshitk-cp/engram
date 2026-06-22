package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/Harshitk-cp/engram/internal/domain"
)

func planIDs() map[string]string {
	return map[string]string{"team": "plan_team", "developer": "plan_dev"}
}

// webhookSig computes a valid X-Razorpay-Signature for a payload.
func webhookSig(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyAndParse_ValidSignature(t *testing.T) {
	secret := "whsec_test"
	c := New("rzp_test", "secret_test", secret, planIDs())
	payload := []byte(`{"event":"subscription.charged","payload":{"subscription":{"entity":{"id":"sub_1","plan_id":"plan_team","customer_id":"cust_1","status":"active","notes":{"tenant_id":"t-123"}}}}}`)
	header := webhookSig(payload, secret)

	ev, err := c.VerifyAndParse(payload, header)
	if err != nil {
		t.Fatalf("expected valid event, got error: %v", err)
	}
	if ev.Type != "subscription.charged" {
		t.Errorf("type = %q", ev.Type)
	}
	if ev.SubscriptionID != "sub_1" || ev.CustomerID != "cust_1" || ev.Status != "active" || ev.PlanID != "plan_team" {
		t.Errorf("unexpected event: %+v", ev)
	}
	if ev.TenantID != "t-123" {
		t.Errorf("tenant id = %q", ev.TenantID)
	}
	if plan, ok := c.PlanForPlanID(ev.PlanID); !ok || plan != domain.PlanTeam {
		t.Errorf("PlanForPlanID = %q, %v", plan, ok)
	}
}

func TestVerifyAndParse_BadSignature(t *testing.T) {
	c := New("rzp_test", "secret_test", "whsec_test", nil)
	payload := []byte(`{"event":"subscription.activated","payload":{"subscription":{"entity":{}}}}`)
	if _, err := c.VerifyAndParse(payload, "deadbeef"); err == nil {
		t.Fatal("expected signature error")
	}
}

func TestVerifyAndParse_NoSecretSkipsVerification(t *testing.T) {
	// With no webhook secret (dev/test), the payload still parses without a sig.
	c := New("rzp_test", "secret_test", "", nil)
	payload := []byte(`{"event":"subscription.cancelled","payload":{"subscription":{"entity":{"id":"sub_9","notes":{"tenant_id":"t-9"}}}}}`)
	ev, err := c.VerifyAndParse(payload, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.SubscriptionID != "sub_9" || ev.TenantID != "t-9" {
		t.Errorf("unexpected event: %+v", ev)
	}
}

func TestVerifyPaymentSignature(t *testing.T) {
	secret := "secret_test"
	c := New("rzp_test", secret, "whsec_test", nil)
	paymentID, subID := "pay_1", "sub_1"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(paymentID + "|" + subID))
	good := hex.EncodeToString(mac.Sum(nil))

	if !c.VerifyPaymentSignature(paymentID, subID, good) {
		t.Error("expected valid payment signature to verify")
	}
	if c.VerifyPaymentSignature(paymentID, subID, "deadbeef") {
		t.Error("expected invalid payment signature to fail")
	}
	if c.VerifyPaymentSignature("", subID, good) {
		t.Error("expected empty payment id to fail")
	}
}

func TestClientDisabledWithoutKeys(t *testing.T) {
	c := New("", "", "", nil)
	if c.Enabled() {
		t.Error("client should be disabled without key id + secret")
	}
	if _, err := c.CreateSubscription(context.Background(), SubscribeParams{Plan: domain.PlanTeam}); err != ErrNotConfigured {
		t.Errorf("expected ErrNotConfigured, got %v", err)
	}
	if err := c.CancelSubscription(context.Background(), "sub_1", true); err != ErrNotConfigured {
		t.Errorf("expected ErrNotConfigured, got %v", err)
	}
}

func TestEnabledRequiresBothKeys(t *testing.T) {
	if New("rzp_test", "", "", nil).Enabled() {
		t.Error("key id alone should not enable the client")
	}
	if New("", "secret_test", "", nil).Enabled() {
		t.Error("key secret alone should not enable the client")
	}
	if !New("rzp_test", "secret_test", "", nil).Enabled() {
		t.Error("key id + secret should enable the client")
	}
}
