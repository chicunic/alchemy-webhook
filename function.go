package function

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
)

func init() {
	functions.HTTP("AlchemyWebhook", AlchemyWebhook)
}

// AlchemyWebhook is the Cloud Run Function entrypoint for Alchemy webhooks
func AlchemyWebhook(w http.ResponseWriter, r *http.Request) {
	signingKey := os.Getenv("ALCHEMY_SIGNING_KEY")
	if signingKey == "" {
		logError("ALCHEMY_SIGNING_KEY environment variable is not set", nil)
		http.Error(w, "Server configuration error", http.StatusInternalServerError)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logError("failed to read request body", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	signature := r.Header.Get("x-alchemy-signature")
	log.Printf(`{"level":"debug","message":"raw webhook received","signature":"%s","body":%s}`, signature, string(body))

	if !verifySignature(body, signature, []byte(signingKey)) {
		logError("signature validation failed", nil)
		http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	webhook, err := parseWebhookEvent(body)
	if err != nil {
		logError("failed to parse webhook event", err)
		http.Error(w, "Invalid webhook event format", http.StatusBadRequest)
		return
	}

	handleWebhook(w, r.Context(), webhook)
}

func verifySignature(body []byte, signature string, signingKey []byte) bool {
	h := hmac.New(sha256.New, signingKey)
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil)) == signature
}

func parseWebhookEvent(body []byte) (*WebhookEvent, error) {
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, err
	}
	return &event, nil
}

func logError(message string, err error) {
	if err != nil {
		log.Printf(`{"level":"error","message":"%s","error":"%s"}`, message, err.Error())
	} else {
		log.Printf(`{"level":"error","message":"%s"}`, message)
	}
}

func handleWebhook(w http.ResponseWriter, ctx context.Context, webhook *WebhookEvent) {
	transfers, err := ParseTransferEvents(webhook)
	if err != nil {
		logError("failed to parse transfer events", err)
		http.Error(w, "Failed to parse transfer events", http.StatusBadRequest)
		return
	}

	if len(transfers) == 0 {
		log.Printf(`{"level":"warn","message":"no transfer events found in webhook","webhook_id":"%s"}`, webhook.WebhookID)
		w.WriteHeader(http.StatusOK)
		return
	}

	transfersJSON, _ := json.Marshal(transfers)
	log.Printf(`{"level":"info","message":"parsed transfer events","webhook_id":"%s","count":%d,"transfers":%s}`,
		webhook.WebhookID, len(transfers), string(transfersJSON))

	if os.Getenv("ENABLE_PUBSUB") == "true" {
		if err := publishToPubSub(ctx, transfers); err != nil {
			logError("failed to publish to Pub/Sub", err)
			http.Error(w, "Failed to publish to Pub/Sub", http.StatusInternalServerError)
			return
		}
	}

	if os.Getenv("ENABLE_FIRESTORE") == "true" {
		if err := writeToFirestore(ctx, transfers); err != nil {
			logError("failed to write to Firestore", err)
			http.Error(w, "Failed to write to Firestore", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func publishToPubSub(ctx context.Context, transfers []*TransferDocument) error {
	publisher, err := NewPubSubPublisher(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err := publisher.Close(); err != nil {
			log.Printf(`{"level":"error","message":"failed to close pubsub publisher","error":"%s"}`, err.Error())
		}
	}()
	return publisher.PublishTransfers(ctx, transfers)
}

func writeToFirestore(ctx context.Context, transfers []*TransferDocument) error {
	writer, err := NewFirestoreWriter(ctx)
	if err != nil {
		return err
	}
	return writer.WriteBatchTransfers(ctx, transfers)
}
