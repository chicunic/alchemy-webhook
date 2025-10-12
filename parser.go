package function

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// Block represents blockchain block information
type Block struct {
	Hash      string `json:"hash"`
	Number    int64  `json:"number"`
	Timestamp int64  `json:"timestamp"`
}

// Transaction represents blockchain transaction information
type Transaction struct {
	Hash     string `json:"hash"`
	From     string `json:"from"`
	To       string `json:"to"`
	Value    string `json:"value"`
	GasPrice string `json:"gasPrice"`
	Gas      int64  `json:"gas"`
	Status   int    `json:"status"`
	GasUsed  int64  `json:"gasUsed"`
}

// Transfer represents ERC20 transfer event information
type Transfer struct {
	Contract string   `json:"contract"`
	From     string   `json:"from"`
	To       string   `json:"to"`
	Value    *big.Int `json:"value"`
	LogIndex int      `json:"logIndex"`
}

// MarshalJSON implements custom JSON marshaling for Transfer
func (t Transfer) MarshalJSON() ([]byte, error) {
	type Alias Transfer
	return json.Marshal(&struct {
		Value string `json:"value"`
		*Alias
	}{
		Value: t.Value.String(),
		Alias: (*Alias)(&t),
	})
}

// UnmarshalJSON implements custom JSON unmarshaling for Transfer
func (t *Transfer) UnmarshalJSON(data []byte) error {
	type Alias Transfer
	aux := &struct {
		Value string `json:"value"`
		*Alias
	}{
		Alias: (*Alias)(t),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.Value != "" {
		t.Value = new(big.Int)
		t.Value.SetString(aux.Value, 10)
	}
	return nil
}

// AlchemyMetadata represents Alchemy-specific metadata
type AlchemyMetadata struct {
	WebhookID      string `json:"webhookId"`
	EventID        string `json:"eventId"`
	SequenceNumber string `json:"sequenceNumber"`
	CreatedAt      string `json:"createdAt"`
}

// TransferDocument represents the complete document structure
type TransferDocument struct {
	Block       Block           `json:"block"`
	Transaction Transaction     `json:"transaction"`
	Transfer    Transfer        `json:"transfer"`
	Network     string          `json:"network"`
	Alchemy     AlchemyMetadata `json:"alchemy"`
}

// WebhookEvent represents the raw webhook event from Alchemy
type WebhookEvent struct {
	WebhookID string    `json:"webhookId"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Type      string    `json:"type"`
	Event     struct {
		Data struct {
			Block struct {
				Hash      string `json:"hash"`
				Number    int64  `json:"number"`
				Timestamp int64  `json:"timestamp"`
				Logs      []struct {
					Data    string   `json:"data"`
					Topics  []string `json:"topics"`
					Index   int      `json:"index"`
					Account struct {
						Address string `json:"address"`
					} `json:"account"`
					Transaction struct {
						Hash string `json:"hash"`
						From struct {
							Address string `json:"address"`
						} `json:"from"`
						To struct {
							Address string `json:"address"`
						} `json:"to"`
						Value    string `json:"value"`
						GasPrice string `json:"gasPrice"`
						Gas      int64  `json:"gas"`
						Status   int    `json:"status"`
						GasUsed  int64  `json:"gasUsed"`
					} `json:"transaction"`
				} `json:"logs"`
			} `json:"block"`
		} `json:"data"`
		SequenceNumber string `json:"sequenceNumber"`
		Network        string `json:"network"`
	} `json:"event"`
}

// transferEventABI is the ABI definition for ERC20 Transfer event
const transferEventABI = `[{
	"anonymous": false,
	"inputs": [
		{"indexed": true, "name": "from", "type": "address"},
		{"indexed": true, "name": "to", "type": "address"},
		{"indexed": false, "name": "value", "type": "uint256"}
	],
	"name": "Transfer",
	"type": "event"
}]`

// transferEventDecoded represents the decoded Transfer event
type transferEventDecoded struct {
	Value *big.Int
}

// ParseTransferEvents parses all webhook logs into TransferDocuments
func ParseTransferEvents(webhook *WebhookEvent) ([]*TransferDocument, error) {
	logs := webhook.Event.Data.Block.Logs
	documents := make([]*TransferDocument, 0, len(logs))

	for i := range logs {
		doc, err := ParseTransferEvent(webhook, i)
		if err != nil {
			// Skip logs that fail to parse (might not be Transfer events)
			continue
		}
		documents = append(documents, doc)
	}

	return documents, nil
}

// ParseTransferEvent parses a webhook log entry into a TransferDocument using go-ethereum ABI decoder
func ParseTransferEvent(webhook *WebhookEvent, logIndex int) (*TransferDocument, error) {
	if logIndex >= len(webhook.Event.Data.Block.Logs) {
		return nil, fmt.Errorf("log index out of range")
	}

	log := webhook.Event.Data.Block.Logs[logIndex]

	// Validate topics
	if len(log.Topics) < 3 {
		return nil, fmt.Errorf("invalid topics length")
	}

	// Parse Transfer event ABI
	transferABI, err := abi.JSON(strings.NewReader(transferEventABI))
	if err != nil {
		return nil, err
	}

	// Convert data to bytes
	dataBytes := common.FromHex(log.Data)

	// Unpack the event (only non-indexed parameters, i.e., value)
	decoded := new(transferEventDecoded)
	err = transferABI.UnpackIntoInterface(decoded, "Transfer", dataBytes)
	if err != nil {
		return nil, err
	}

	// Extract indexed parameters from topics
	// topics[0] is event signature, topics[1] is from, topics[2] is to
	fromAddr := common.HexToAddress(log.Topics[1])
	toAddr := common.HexToAddress(log.Topics[2])

	doc := &TransferDocument{
		Block: Block{
			Hash:      webhook.Event.Data.Block.Hash,
			Number:    webhook.Event.Data.Block.Number,
			Timestamp: webhook.Event.Data.Block.Timestamp,
		},
		Transaction: Transaction{
			Hash:     log.Transaction.Hash,
			From:     log.Transaction.From.Address,
			To:       log.Transaction.To.Address,
			Value:    convertHexToDecimal(log.Transaction.Value),
			GasPrice: log.Transaction.GasPrice,
			Gas:      log.Transaction.Gas,
			Status:   log.Transaction.Status,
			GasUsed:  log.Transaction.GasUsed,
		},
		Transfer: Transfer{
			Contract: log.Account.Address,
			From:     fromAddr.Hex(),
			To:       toAddr.Hex(),
			Value:    decoded.Value,
			LogIndex: log.Index,
		},
		Network: webhook.Event.Network,
		Alchemy: AlchemyMetadata{
			WebhookID:      webhook.WebhookID,
			EventID:        webhook.ID,
			SequenceNumber: webhook.Event.SequenceNumber,
			CreatedAt:      webhook.CreatedAt.Format(time.RFC3339),
		},
	}

	return doc, nil
}

// convertHexToDecimal converts hex string to decimal string
func convertHexToDecimal(hexStr string) string {
	hexStr = strings.TrimPrefix(hexStr, "0x")
	if hexStr == "" {
		return "0"
	}

	value := new(big.Int)
	value.SetString(hexStr, 16)
	return value.String()
}

// GetDocumentID generates document ID from transaction hash and log index
func GetDocumentID(txHash string, logIndex int) string {
	return fmt.Sprintf("%s-%d", txHash, logIndex)
}
