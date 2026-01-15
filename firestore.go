package function

import (
	"context"
	"log"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go"
)

const (
	collectionName = "alchemy_stream"
	batchLimit     = 500
)

// FirestoreWriter handles writing webhook events to Google Cloud Firestore.
type FirestoreWriter struct {
	app *firebase.App
}

// NewFirestoreWriter creates a new Firestore writer using Firebase Admin SDK.
func NewFirestoreWriter(ctx context.Context) (*FirestoreWriter, error) {
	app, err := firebase.NewApp(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &FirestoreWriter{app: app}, nil
}

// WriteBatchTransfers writes multiple TransferDocuments to Firestore using transactions.
// Ensures atomicity per batch - either all writes succeed or none are applied.
func (f *FirestoreWriter) WriteBatchTransfers(ctx context.Context, transfers []*TransferDocument) error {
	client, err := f.app.Firestore(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err := client.Close(); err != nil {
			log.Printf(`{"level":"error","message":"failed to close firestore client","error":"%s"}`, err.Error())
		}
	}()

	total := len(transfers)
	for start := 0; start < total; start += batchLimit {
		end := min(start+batchLimit, total)
		batch := transfers[start:end]

		err = client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
			for _, transfer := range batch {
				docID := GetDocumentID(transfer.Transaction.Hash, transfer.Transfer.LogIndex)
				docRef := client.Collection(collectionName).Doc(docID)
				if err := tx.Set(docRef, transfer); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}

		log.Printf(`{"level":"info","message":"batch written to firestore","collection":"%s","range":"%d-%d","size":%d}`,
			collectionName, start, end, len(batch))
	}

	log.Printf(`{"level":"info","message":"all batches written to firestore","collection":"%s","total":%d}`,
		collectionName, total)
	return nil
}
