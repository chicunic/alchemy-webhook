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

// Block represents blockchain block information.
type Block struct {
	Hash      string `json:"hash"`
	Number    int64  `json:"number"`
	Timestamp int64  `json:"timestamp"`
}

// Transaction represents blockchain transaction information.
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

// Transfer represents ERC20 transfer event information.
type Transfer struct {
	Contract string   `json:"contract"`
	From     string   `json:"from"`
	To       string   `json:"to"`
	Value    *big.Int `json:"value"`
	LogIndex int      `json:"logIndex"`
}

// transferJSON is used for JSON serialization of Transfer.
type transferJSON struct {
	Contract string `json:"contract"`
	From     string `json:"from"`
	To       string `json:"to"`
	Value    string `json:"value"`
	LogIndex int    `json:"logIndex"`
}

func (t Transfer) MarshalJSON() ([]byte, error) {
	return json.Marshal(transferJSON{
		Contract: t.Contract,
		From:     t.From,
		To:       t.To,
		Value:    t.Value.String(),
		LogIndex: t.LogIndex,
	})
}

func (t *Transfer) UnmarshalJSON(data []byte) error {
	var aux transferJSON
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	t.Contract = aux.Contract
	t.From = aux.From
	t.To = aux.To
	t.LogIndex = aux.LogIndex
	if aux.Value != "" {
		t.Value = new(big.Int)
		t.Value.SetString(aux.Value, 10)
	}
	return nil
}

// AlchemyMetadata represents Alchemy-specific metadata.
type AlchemyMetadata struct {
	WebhookID      string `json:"webhookId"`
	EventID        string `json:"eventId"`
	SequenceNumber string `json:"sequenceNumber"`
	CreatedAt      string `json:"createdAt"`
}

// TransferDocument represents the complete document structure.
type TransferDocument struct {
	Block       Block           `json:"block"`
	Transaction Transaction     `json:"transaction"`
	Transfer    Transfer        `json:"transfer"`
	Network     string          `json:"network"`
	Alchemy     AlchemyMetadata `json:"alchemy"`
}

// WebhookLog represents a single log entry in the webhook event.
type WebhookLog struct {
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
}

// WebhookEvent represents the raw webhook event from Alchemy.
type WebhookEvent struct {
	WebhookID string    `json:"webhookId"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Type      string    `json:"type"`
	Event     struct {
		Data struct {
			Block struct {
				Hash      string       `json:"hash"`
				Number    int64        `json:"number"`
				Timestamp int64        `json:"timestamp"`
				Logs      []WebhookLog `json:"logs"`
			} `json:"block"`
		} `json:"data"`
		SequenceNumber string `json:"sequenceNumber"`
		Network        string `json:"network"`
	} `json:"event"`
}

// ERC20 Transfer event ABI definition.
const transferEventABI = `[{"anonymous":false,"inputs":[{"indexed":true,"name":"from","type":"address"},{"indexed":true,"name":"to","type":"address"},{"indexed":false,"name":"value","type":"uint256"}],"name":"Transfer","type":"event"}]`

var parsedTransferABI abi.ABI

func init() {
	var err error
	parsedTransferABI, err = abi.JSON(strings.NewReader(transferEventABI))
	if err != nil {
		panic("failed to parse transfer event ABI: " + err.Error())
	}
}

// decodedTransferEvent holds the decoded value from Transfer event data.
type decodedTransferEvent struct {
	Value *big.Int
}

// ParseTransferEvents parses all webhook logs into TransferDocuments.
func ParseTransferEvents(webhook *WebhookEvent) ([]*TransferDocument, error) {
	logs := webhook.Event.Data.Block.Logs
	documents := make([]*TransferDocument, 0, len(logs))

	for i := range logs {
		doc, err := parseLogEntry(webhook, i)
		if err != nil {
			continue // Skip non-Transfer events
		}
		documents = append(documents, doc)
	}

	return documents, nil
}

// parseLogEntry parses a single log entry into a TransferDocument.
func parseLogEntry(webhook *WebhookEvent, index int) (*TransferDocument, error) {
	logs := webhook.Event.Data.Block.Logs
	if index >= len(logs) {
		return nil, fmt.Errorf("log index out of range")
	}

	log := logs[index]
	if len(log.Topics) < 3 {
		return nil, fmt.Errorf("invalid topics length")
	}

	var decoded decodedTransferEvent
	if err := parsedTransferABI.UnpackIntoInterface(&decoded, "Transfer", common.FromHex(log.Data)); err != nil {
		return nil, err
	}

	block := webhook.Event.Data.Block
	return &TransferDocument{
		Block: Block{
			Hash:      block.Hash,
			Number:    block.Number,
			Timestamp: block.Timestamp,
		},
		Transaction: Transaction{
			Hash:     log.Transaction.Hash,
			From:     log.Transaction.From.Address,
			To:       log.Transaction.To.Address,
			Value:    hexToDecimal(log.Transaction.Value),
			GasPrice: log.Transaction.GasPrice,
			Gas:      log.Transaction.Gas,
			Status:   log.Transaction.Status,
			GasUsed:  log.Transaction.GasUsed,
		},
		Transfer: Transfer{
			Contract: log.Account.Address,
			From:     common.HexToAddress(log.Topics[1]).Hex(),
			To:       common.HexToAddress(log.Topics[2]).Hex(),
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
	}, nil
}

// hexToDecimal converts a hex string to its decimal representation.
func hexToDecimal(hex string) string {
	hex = strings.TrimPrefix(hex, "0x")
	if hex == "" {
		return "0"
	}
	value := new(big.Int)
	value.SetString(hex, 16)
	return value.String()
}

// GetDocumentID generates a document ID from transaction hash and log index.
func GetDocumentID(txHash string, logIndex int) string {
	return fmt.Sprintf("%s-%d", txHash, logIndex)
}
