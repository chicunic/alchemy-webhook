package function

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"

	"cloud.google.com/go/pubsub/v2"
)

var (
	pubsubOnce      sync.Once
	pubsubPublisher *PubSubPublisher
	pubsubErr       error
)

// getPubSubPublisher lazily initializes and caches a shared PubSubPublisher.
// The underlying client is reused across invocations to avoid per-request
// connection setup overhead.
func getPubSubPublisher(ctx context.Context) (*PubSubPublisher, error) {
	pubsubOnce.Do(func() {
		pubsubPublisher, pubsubErr = NewPubSubPublisher(ctx)
	})
	return pubsubPublisher, pubsubErr
}

// PubSubPublisher handles publishing webhook events to Google Cloud Pub/Sub.
type PubSubPublisher struct {
	client    *pubsub.Client
	publisher *pubsub.Publisher
}

// NewPubSubPublisher creates a new Pub/Sub publisher.
func NewPubSubPublisher(ctx context.Context) (*PubSubPublisher, error) {
	projectID := getProjectID()
	if projectID == "" {
		return nil, errors.New("project ID not found (GCP_PROJECT or GOOGLE_CLOUD_PROJECT)")
	}

	topicID := os.Getenv("ALCHEMY_PUBSUB_TOPIC")
	if topicID == "" {
		return nil, errors.New("ALCHEMY_PUBSUB_TOPIC environment variable is not set")
	}

	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, err
	}

	return &PubSubPublisher{
		client:    client,
		publisher: client.Publisher(topicID),
	}, nil
}

func getProjectID() string {
	if id := os.Getenv("GCP_PROJECT"); id != "" {
		return id
	}
	return os.Getenv("GOOGLE_CLOUD_PROJECT")
}

// PublishTransfers publishes an array of TransferDocuments to Pub/Sub as a single message.
func (p *PubSubPublisher) PublishTransfers(ctx context.Context, transfers []*TransferDocument) error {
	data, err := json.Marshal(transfers)
	if err != nil {
		return fmt.Errorf("failed to marshal transfers: %w", err)
	}

	result := p.publisher.Publish(ctx, &pubsub.Message{
		Data:       data,
		Attributes: buildAttributes(transfers),
	})

	messageID, err := result.Get(ctx)
	if err != nil {
		return err
	}

	slog.Info("published transfers to pubsub", "message_id", messageID, "count", len(transfers))
	return nil
}

func buildAttributes(transfers []*TransferDocument) map[string]string {
	if len(transfers) == 0 {
		return map[string]string{"count": "0"}
	}
	first := transfers[0]
	return map[string]string{
		"webhook_id": first.Alchemy.WebhookID,
		"event_id":   first.Alchemy.EventID,
		"network":    first.Network,
		"count":      strconv.Itoa(len(transfers)),
	}
}
