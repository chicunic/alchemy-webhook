package function

import (
	"context"
	"log"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go"
)

// FirestoreWriter handles writing webhook events to Google Cloud Firestore
type FirestoreWriter struct {
	app        *firebase.App
	collection string
}

// NewFirestoreWriter creates a new Firestore writer using Firebase Admin SDK
func NewFirestoreWriter(ctx context.Context) (*FirestoreWriter, error) {
	// Use application default credentials
	// Project ID will be auto-detected from the environment in Cloud Functions
	app, err := firebase.NewApp(ctx, nil)
	if err != nil {
		return nil, err
	}

	return &FirestoreWriter{
		app:        app,
		collection: "alchemy_stream",
	}, nil
}

// WriteBatchTransfers writes multiple TransferDocuments to Firestore using transactions
// This ensures atomicity - either all writes succeed or none are applied, making retries safe
// For batches > 500 items, splits into multiple transactions (Firestore limit)
func (f *FirestoreWriter) WriteBatchTransfers(ctx context.Context, transfers []*TransferDocument) error {
	client, err := f.app.Firestore(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	const maxBatchSize = 500 // Firestore transaction limit
	totalCount := len(transfers)

	// Process in batches of 500
	for i := 0; i < totalCount; i += maxBatchSize {
		end := i + maxBatchSize
		if end > totalCount {
			end = totalCount
		}
		batch := transfers[i:end]

		err = client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
			for _, transfer := range batch {
				docID := GetDocumentID(transfer.Transaction.Hash, transfer.Transfer.LogIndex)
				docRef := client.Collection(f.collection).Doc(docID)
				if err := tx.Set(docRef, transfer); err != nil {
					return err
				}
			}
			return nil
		})

		if err != nil {
			return err
		}

		log.Printf(`{"level":"info","message":"batch written to firestore","collection":"%s","batch_range":"%d-%d","batch_size":%d}`, f.collection, i, end, len(batch))
	}

	log.Printf(`{"level":"info","message":"all batches written to firestore","collection":"%s","total_count":%d}`, f.collection, totalCount)
	return nil
}
