package function

import (
	"encoding/json"
	"math/big"
	"testing"
	"time"
)

func TestHexToDecimal(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"0x0", "0"},
		{"0x1", "1"},
		{"0xa", "10"},
		{"0xff", "255"},
		{"0x38d7ea4c68000", "1000000000000000"},
		{"", "0"},
		{"0x", "0"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := hexToDecimal(tt.input)
			if got != tt.want {
				t.Errorf("hexToDecimal(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetDocumentID(t *testing.T) {
	tests := []struct {
		hash     string
		logIndex int
		want     string
	}{
		{"0xabc123", 0, "0xabc123-0"},
		{"0xabc123", 5, "0xabc123-5"},
		{"0xdef456", 100, "0xdef456-100"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := GetDocumentID(tt.hash, tt.logIndex)
			if got != tt.want {
				t.Errorf("GetDocumentID(%q, %d) = %q, want %q", tt.hash, tt.logIndex, got, tt.want)
			}
		})
	}
}

func TestTransferJSONRoundtrip(t *testing.T) {
	original := Transfer{
		Contract: "0xdAC17F958D2ee523a2206206994597C13D831ec7",
		From:     "0xSender",
		To:       "0xReceiver",
		Value:    big.NewInt(1000000),
		LogIndex: 3,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Verify JSON contains string value, not number
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal raw error: %v", err)
	}
	if _, ok := raw["value"].(string); !ok {
		t.Errorf("expected value to be string in JSON, got %T", raw["value"])
	}

	var decoded Transfer
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Contract != original.Contract {
		t.Errorf("Contract = %q, want %q", decoded.Contract, original.Contract)
	}
	if decoded.Value.Cmp(original.Value) != 0 {
		t.Errorf("Value = %s, want %s", decoded.Value, original.Value)
	}
	if decoded.LogIndex != original.LogIndex {
		t.Errorf("LogIndex = %d, want %d", decoded.LogIndex, original.LogIndex)
	}
}

func TestParseTransferEvents(t *testing.T) {
	// ERC20 Transfer(address,address,uint256) topic
	transferTopic := "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
	fromTopic := "0x000000000000000000000000aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	toTopic := "0x000000000000000000000000bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	// value = 1000 (0x3e8) padded to 32 bytes
	dataHex := "0x00000000000000000000000000000000000000000000000000000000000003e8"

	webhook := &WebhookEvent{
		WebhookID: "wh_test",
		ID:        "evt_test",
		CreatedAt: time.Now(),
		Type:      "ADDRESS_ACTIVITY",
	}
	webhook.Event.SequenceNumber = "1"
	webhook.Event.Network = "ETH_MAINNET"
	webhook.Event.Data.Block.Hash = "0xblockhash"
	webhook.Event.Data.Block.Number = 12345
	webhook.Event.Data.Block.Timestamp = 1700000000
	webhook.Event.Data.Block.Logs = []WebhookLog{
		{
			Data:   dataHex,
			Topics: []string{transferTopic, fromTopic, toTopic},
			Index:  0,
			Account: struct {
				Address string `json:"address"`
			}{Address: "0xContractAddress"},
			Transaction: struct {
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
			}{
				Hash:     "0xtxhash",
				From:     struct{ Address string `json:"address"` }{Address: "0xTxFrom"},
				To:       struct{ Address string `json:"address"` }{Address: "0xTxTo"},
				Value:    "0x0",
				GasPrice: "0x3b9aca00",
				Gas:      21000,
				Status:   1,
				GasUsed:  21000,
			},
		},
		{
			// Non-transfer log (only 1 topic)
			Data:   "0x",
			Topics: []string{"0xothertopic"},
			Index:  1,
		},
	}

	docs, err := ParseTransferEvents(webhook)
	if err != nil {
		t.Fatalf("ParseTransferEvents() error = %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}

	doc := docs[0]
	if doc.Network != "ETH_MAINNET" {
		t.Errorf("Network = %q, want %q", doc.Network, "ETH_MAINNET")
	}
	if doc.Block.Number != 12345 {
		t.Errorf("Block.Number = %d, want %d", doc.Block.Number, 12345)
	}
	if doc.Transfer.Value.Cmp(big.NewInt(1000)) != 0 {
		t.Errorf("Transfer.Value = %s, want 1000", doc.Transfer.Value)
	}
	if doc.Transfer.LogIndex != 0 {
		t.Errorf("Transfer.LogIndex = %d, want 0", doc.Transfer.LogIndex)
	}
	if doc.Alchemy.WebhookID != "wh_test" {
		t.Errorf("Alchemy.WebhookID = %q, want %q", doc.Alchemy.WebhookID, "wh_test")
	}
}

func TestParseTransferEventsEmpty(t *testing.T) {
	webhook := &WebhookEvent{}
	docs, err := ParseTransferEvents(webhook)
	if err != nil {
		t.Fatalf("ParseTransferEvents() error = %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("expected 0 documents, got %d", len(docs))
	}
}

func TestBuildAttributes(t *testing.T) {
	t.Run("empty transfers", func(t *testing.T) {
		attrs := buildAttributes(nil)
		if attrs["count"] != "0" {
			t.Errorf("count = %q, want %q", attrs["count"], "0")
		}
	})

	t.Run("with transfers", func(t *testing.T) {
		transfers := []*TransferDocument{
			{
				Network: "ETH_MAINNET",
				Alchemy: AlchemyMetadata{
					WebhookID: "wh_123",
					EventID:   "evt_456",
				},
			},
			{
				Network: "ETH_MAINNET",
				Alchemy: AlchemyMetadata{
					WebhookID: "wh_123",
					EventID:   "evt_456",
				},
			},
		}
		attrs := buildAttributes(transfers)
		if attrs["webhook_id"] != "wh_123" {
			t.Errorf("webhook_id = %q, want %q", attrs["webhook_id"], "wh_123")
		}
		if attrs["event_id"] != "evt_456" {
			t.Errorf("event_id = %q, want %q", attrs["event_id"], "evt_456")
		}
		if attrs["network"] != "ETH_MAINNET" {
			t.Errorf("network = %q, want %q", attrs["network"], "ETH_MAINNET")
		}
		if attrs["count"] != "2" {
			t.Errorf("count = %q, want %q", attrs["count"], "2")
		}
	})
}
