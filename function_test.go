package function

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

func computeTestHMAC(body, key []byte) string {
	h := hmac.New(sha256.New, key)
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"test":"data"}`)
	signingKey := []byte("test-key")
	validSig := computeTestHMAC(body, signingKey)

	tests := []struct {
		name      string
		body      []byte
		signature string
		key       []byte
		want      bool
	}{
		{"valid signature", body, validSig, signingKey, true},
		{"invalid signature", body, "invalid", signingKey, false},
		{"empty signature", body, "", signingKey, false},
		{"wrong key", body, validSig, []byte("wrong-key"), false},
		{"empty body", []byte{}, computeTestHMAC([]byte{}, signingKey), signingKey, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := verifySignature(tt.body, tt.signature, tt.key)
			if got != tt.want {
				t.Errorf("verifySignature() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseWebhookEvent(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	event := WebhookEvent{
		WebhookID: "wh_123",
		ID:        "evt_456",
		CreatedAt: now,
		Type:      "ADDRESS_ACTIVITY",
	}
	event.Event.SequenceNumber = "1"
	event.Event.Network = "ETH_MAINNET"

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal test event: %v", err)
	}

	parsed, err := parseWebhookEvent(data)
	if err != nil {
		t.Fatalf("parseWebhookEvent() error = %v", err)
	}

	if parsed.WebhookID != "wh_123" {
		t.Errorf("WebhookID = %q, want %q", parsed.WebhookID, "wh_123")
	}
	if parsed.ID != "evt_456" {
		t.Errorf("ID = %q, want %q", parsed.ID, "evt_456")
	}
	if parsed.Event.Network != "ETH_MAINNET" {
		t.Errorf("Network = %q, want %q", parsed.Event.Network, "ETH_MAINNET")
	}
}

func TestParseWebhookEventInvalidJSON(t *testing.T) {
	_, err := parseWebhookEvent([]byte(`{invalid`))
	if err == nil {
		t.Error("parseWebhookEvent() expected error for invalid JSON")
	}
}
