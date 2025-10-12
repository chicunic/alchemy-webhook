package function

import (
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

func jsonToWebhookEvent(body []byte) (*WebhookEvent, error) {
	event := new(WebhookEvent)
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, err
	}
	return event, nil
}

func isValidSignatureForStringBody(
	body []byte,
	signature string,
	signingKey []byte,
) bool {
	h := hmac.New(sha256.New, signingKey)
	h.Write(body)
	digest := hex.EncodeToString(h.Sum(nil))
	return digest == signature
}

// AlchemyWebhook is the Cloud Run Function entrypoint for Alchemy webhooks
func AlchemyWebhook(w http.ResponseWriter, r *http.Request) {
	// Get signing key from environment
	signingKey := os.Getenv("ALCHEMY_SIGNING_KEY")
	if signingKey == "" {
		log.Printf(`{"level":"error","message":"ALCHEMY_SIGNING_KEY environment variable is not set"}`)
		http.Error(w, "Server configuration error", http.StatusInternalServerError)
		return
	}

	signature := r.Header.Get("x-alchemy-signature")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf(`{"level":"error","message":"failed to read request body","error":"%s"}`, err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	r.Body.Close()

	// Log raw JSON for debugging in structured format
	log.Printf(`{"level":"debug","message":"raw webhook received","signature":"%s","body":%s}`, signature, string(body))

	isValidSignature := isValidSignatureForStringBody(body, signature, []byte(signingKey))
	if !isValidSignature {
		log.Printf(`{"level":"error","message":"signature validation failed","signature":"%s"}`, signature)
		http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	webhook, err := jsonToWebhookEvent(body)
	if err != nil {
		log.Printf(`{"level":"error","message":"failed to parse webhook event","error":"%s"}`, err.Error())
		http.Error(w, "Invalid webhook event format", http.StatusBadRequest)
		return
	}

	handleAlchemyWebhook(w, r, webhook)
}

func init() {
	functions.HTTP("AlchemyWebhook", AlchemyWebhook)
}

// handleAlchemyWebhook processes the webhook event
func handleAlchemyWebhook(w http.ResponseWriter, r *http.Request, webhook *WebhookEvent) {
	ctx := r.Context()

	// Parse transfer events from webhook
	transfers, err := ParseTransferEvents(webhook)
	if err != nil {
		log.Printf(`{"level":"error","message":"failed to parse transfer events","error":"%s"}`, err.Error())
		http.Error(w, "Failed to parse transfer events", http.StatusBadRequest)
		return
	}

	if len(transfers) == 0 {
		log.Printf(`{"level":"warn","message":"no transfer events found in webhook","webhook_id":"%s"}`, webhook.WebhookID)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Log parsed transfers with full details
	transfersJSON, _ := json.Marshal(transfers)
	log.Printf(`{"level":"info","message":"parsed transfer events","webhook_id":"%s","count":%d,"transfers":%s}`, webhook.WebhookID, len(transfers), string(transfersJSON))

	// Publish to Pub/Sub (blocking, ensures message delivery)
	if os.Getenv("ENABLE_PUBSUB") == "true" {
		publisher, err := NewPubSubPublisher(ctx)
		if err != nil {
			log.Printf(`{"level":"error","message":"failed to create pubsub publisher","error":"%s"}`, err.Error())
			http.Error(w, "Failed to initialize Pub/Sub", http.StatusInternalServerError)
			return
		}
		defer publisher.Close()

		if err := publisher.PublishTransfers(ctx, transfers); err != nil {
			log.Printf(`{"level":"error","message":"failed to publish transfers","error":"%s"}`, err.Error())
			http.Error(w, "Failed to publish to Pub/Sub", http.StatusInternalServerError)
			return
		}
	}

	// Write to Firestore (blocking, ensures data persistence)
	if os.Getenv("ENABLE_FIRESTORE") == "true" {
		writer, err := NewFirestoreWriter(ctx)
		if err != nil {
			log.Printf(`{"level":"error","message":"failed to create firestore writer","error":"%s"}`, err.Error())
			http.Error(w, "Failed to initialize Firestore", http.StatusInternalServerError)
			return
		}

		if err := writer.WriteBatchTransfers(ctx, transfers); err != nil {
			log.Printf(`{"level":"error","message":"failed to write batch to firestore","error":"%s"}`, err.Error())
			http.Error(w, "Failed to write to Firestore", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}
