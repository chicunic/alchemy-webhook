package function

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
)

func init() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	functions.HTTP("AlchemyWebhook", AlchemyWebhook)
}

// AlchemyWebhook is the Cloud Run Function entrypoint for Alchemy webhooks
func AlchemyWebhook(w http.ResponseWriter, r *http.Request) {
	signingKey := os.Getenv("ALCHEMY_SIGNING_KEY")
	if signingKey == "" {
		slog.Error("ALCHEMY_SIGNING_KEY environment variable is not set")
		http.Error(w, "Server configuration error", http.StatusInternalServerError)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("failed to read request body", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	signature := r.Header.Get("x-alchemy-signature")
	slog.Debug("raw webhook received", "body", string(body))

	if !verifySignature(body, signature, []byte(signingKey)) {
		slog.Error("signature validation failed")
		http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	webhook, err := parseWebhookEvent(body)
	if err != nil {
		slog.Error("failed to parse webhook event", "error", err)
		http.Error(w, "Invalid webhook event format", http.StatusBadRequest)
		return
	}

	handleWebhook(w, r.Context(), webhook)
}

func verifySignature(body []byte, signature string, signingKey []byte) bool {
	h := hmac.New(sha256.New, signingKey)
	h.Write(body)
	expected := h.Sum(nil)

	sig, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	return hmac.Equal(expected, sig)
}

func parseWebhookEvent(body []byte) (*WebhookEvent, error) {
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, err
	}
	return &event, nil
}

func handleWebhook(w http.ResponseWriter, ctx context.Context, webhook *WebhookEvent) {
	transfers, err := ParseTransferEvents(webhook)
	if err != nil {
		slog.Error("failed to parse transfer events", "error", err)
		http.Error(w, "Failed to parse transfer events", http.StatusBadRequest)
		return
	}

	if len(transfers) == 0 {
		slog.Warn("no transfer events found in webhook", "webhook_id", webhook.WebhookID)
		w.WriteHeader(http.StatusOK)
		return
	}

	transfersJSON, err := json.Marshal(transfers)
	if err != nil {
		slog.Error("failed to marshal transfer events", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	slog.Info("parsed transfer events", "webhook_id", webhook.WebhookID, "count", len(transfers), "transfers", string(transfersJSON))

	if os.Getenv("ENABLE_PUBSUB") == "true" {
		if err := publishToPubSub(ctx, transfers); err != nil {
			slog.Error("failed to publish to Pub/Sub", "error", err)
			http.Error(w, "Failed to publish to Pub/Sub", http.StatusInternalServerError)
			return
		}
	}

	if os.Getenv("ENABLE_FIRESTORE") == "true" {
		if err := writeToFirestore(ctx, transfers); err != nil {
			slog.Error("failed to write to Firestore", "error", err)
			http.Error(w, "Failed to write to Firestore", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func publishToPubSub(ctx context.Context, transfers []*TransferDocument) error {
	publisher, err := getPubSubPublisher(ctx)
	if err != nil {
		return err
	}
	return publisher.PublishTransfers(ctx, transfers)
}

func writeToFirestore(ctx context.Context, transfers []*TransferDocument) error {
	writer, err := getFirestoreWriter(ctx)
	if err != nil {
		return err
	}
	return writer.WriteBatchTransfers(ctx, transfers)
}
