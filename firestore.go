package function

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"sync"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
)

var (
	firestoreCollection = getEnvOrDefault("FIRESTORE_COLLECTION", "alchemy_stream")
	firestoreBatchLimit = getEnvOrDefaultInt("FIRESTORE_BATCH_LIMIT", 500)
)

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getEnvOrDefaultInt(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultValue
}

var (
	firestoreOnce   sync.Once
	firestoreWriter *FirestoreWriter
	firestoreErr    error
)

// getFirestoreWriter lazily initializes and caches a shared FirestoreWriter.
// The underlying client is reused across invocations to avoid per-request
// connection setup overhead.
func getFirestoreWriter(ctx context.Context) (*FirestoreWriter, error) {
	firestoreOnce.Do(func() {
		firestoreWriter, firestoreErr = NewFirestoreWriter(ctx)
	})
	return firestoreWriter, firestoreErr
}

// FirestoreWriter handles writing webhook events to Google Cloud Firestore.
type FirestoreWriter struct {
	client *firestore.Client
}

// NewFirestoreWriter creates a new Firestore writer using Firebase Admin SDK.
func NewFirestoreWriter(ctx context.Context) (*FirestoreWriter, error) {
	app, err := firebase.NewApp(ctx, nil)
	if err != nil {
		return nil, err
	}
	client, err := app.Firestore(ctx)
	if err != nil {
		return nil, err
	}
	return &FirestoreWriter{client: client}, nil
}

// WriteBatchTransfers writes multiple TransferDocuments to Firestore using transactions.
// Ensures atomicity per batch - either all writes succeed or none are applied.
func (f *FirestoreWriter) WriteBatchTransfers(ctx context.Context, transfers []*TransferDocument) error {
	client := f.client

	total := len(transfers)
	for start := 0; start < total; start += firestoreBatchLimit {
		end := min(start+firestoreBatchLimit, total)
		batch := transfers[start:end]

		err := client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
			for _, transfer := range batch {
				docID := GetDocumentID(transfer.Transaction.Hash, transfer.Transfer.LogIndex)
				docRef := client.Collection(firestoreCollection).Doc(docID)
				if err := tx.Set(docRef, transfer); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}

		slog.Info("batch written to firestore", "collection", firestoreCollection, "start", start, "end", end, "size", len(batch))
	}

	slog.Info("all batches written to firestore", "collection", firestoreCollection, "total", total)
	return nil
}
