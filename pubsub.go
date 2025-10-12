package function

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"cloud.google.com/go/pubsub/v2"
)

// PubSubPublisher handles publishing webhook events to Google Cloud Pub/Sub
type PubSubPublisher struct {
	client    *pubsub.Client
	publisher *pubsub.Publisher
	topicID   string
}

// NewPubSubPublisher creates a new Pub/Sub publisher
func NewPubSubPublisher(ctx context.Context) (*PubSubPublisher, error) {
	// Get project ID from environment variables
	// Cloud Functions automatically provides GOOGLE_CLOUD_PROJECT
	projectID := os.Getenv("GCP_PROJECT")
	if projectID == "" {
		projectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}
	if projectID == "" {
		return nil, fmt.Errorf("project ID not found in environment variables (GCP_PROJECT or GOOGLE_CLOUD_PROJECT)")
	}

	topicID := os.Getenv("ALCHEMY_PUBSUB_TOPIC")
	if topicID == "" {
		return nil, fmt.Errorf("ALCHEMY_PUBSUB_TOPIC environment variable is not set")
	}

	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, err
	}

	publisher := client.Publisher(topicID)

	return &PubSubPublisher{
		client:    client,
		publisher: publisher,
		topicID:   topicID,
	}, nil
}

// PublishTransfers publishes an array of TransferDocuments to Pub/Sub as a single message
func (p *PubSubPublisher) PublishTransfers(ctx context.Context, transfers []*TransferDocument) error {
	data, _ := json.Marshal(transfers)

	// Use first transfer's metadata for message attributes
	var attributes map[string]string
	if len(transfers) > 0 {
		attributes = map[string]string{
			"webhook_id": transfers[0].Alchemy.WebhookID,
			"event_id":   transfers[0].Alchemy.EventID,
			"network":    transfers[0].Network,
			"count":      fmt.Sprintf("%d", len(transfers)),
		}
	} else {
		attributes = map[string]string{
			"count": "0",
		}
	}

	result := p.publisher.Publish(ctx, &pubsub.Message{
		Data:       data,
		Attributes: attributes,
	})

	messageID, err := result.Get(ctx)
	if err != nil {
		return err
	}

	log.Printf(`{"level":"info","message":"published transfers to pubsub","message_id":"%s","count":%d}`, messageID, len(transfers))
	return nil
}

// Close closes the Pub/Sub client
func (p *PubSubPublisher) Close() error {
	// Stop the publisher to flush any pending messages
	p.publisher.Stop()
	return p.client.Close()
}
