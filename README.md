# Alchemy Webhook

Alchemy webhook tool for receiving and processing ERC20 Token Transfer events.

## Features

1. **Webhook Signature Verification** - Secure HMAC-SHA256 signature validation
2. **Cloud Function** - Receive and process webhook events
3. **Pub/Sub Integration** - Reliable message publishing with automatic retries
4. **Firestore Storage** - Transactional persistence with automatic batch handling (up to 500 documents per transaction)

## GraphQL Query

This project specifically monitors ERC20 Transfer events using the following GraphQL query:

```graphql
{
  block {
    hash
    number
    timestamp
    logs(filter: {
      addresses: ["<YOUR_ERC20_CONTRACT_ADDRESS>"]
      topics: ["0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"]
    }) {
      data
      topics
      index
      account {
        address
      }
      transaction {
        hash
        from { address }
        to { address }
        value
        gasPrice
        gas
        status
        gasUsed
      }
    }
  }
}
```

**Notes:**

- `addresses` - ERC20 contract address to monitor
- `topics[0]` - ERC20 Transfer event signature `0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef`
  - This is the Keccak-256 hash of `Transfer(address,address,uint256)` event
  - All ERC20-compliant token contracts use the same event signature
  - `topics[1]` is `from` address, `topics[2]` is `to` address, `data` is transfer amount

## Webhook Event Example

Received event format:

```json
{
  "webhookId": "wh_xxxxx",
  "id": "whevt_xxxxx",
  "createdAt": "2026-01-01T00:00:00.000Z",
  "type": "GRAPHQL",
  "event": {
    "data": {
      "block": {
        "hash": "0x...",
        "number": 123456,
        "timestamp": 1234567890,
        "logs": [
          {
            "data": "0x...",
            "topics": [
              "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
              "0x000000000000000000000000<FROM_ADDRESS>",
              "0x000000000000000000000000<TO_ADDRESS>"
            ],
            "index": 0,
            "account": { "address": "0x..." },
            "transaction": {
              "hash": "0x...",
              "from": { "address": "0x..." },
              "to": { "address": "0x..." },
              "value": "0x0",
              "gasPrice": "0x...",
              "gas": 21000,
              "status": 1,
              "gasUsed": 21000
            }
          }
        ]
      }
    },
    "sequenceNumber": "...",
    "network": "ETH_MAINNET"
  }
}
```

## Quick Start

### Deploy Cloud Function

Deploy using Cloud Build:

```bash
gcloud beta builds submit --config cloudbuild.yaml
```

## Environment Variables

```bash
# Required
ALCHEMY_SIGNING_KEY=your_signing_key_here

# Optional
ENABLE_PUBSUB=true
ALCHEMY_PUBSUB_TOPIC=your-topic-id
ENABLE_FIRESTORE=true
```

## Data Processing

Each Transfer Event is processed into a separate document containing block information, transaction details, and transfer information:

```json
{
  "block": {
    "hash": "0x...",
    "number": 123456,
    "timestamp": 1234567890
  },
  "transaction": {
    "hash": "0x...",
    "from": "0x...",
    "to": "0x...",
    "value": "0",
    "gasPrice": "0x...",
    "gas": 21000,
    "status": 1,
    "gasUsed": 21000
  },
  "transfer": {
    "contract": "0x...",
    "from": "0x...",
    "to": "0x...",
    "value": "1000000000000000000",
    "logIndex": 0
  },
  "network": "ETH_SEPOLIA",
  "alchemy": {
    "webhookId": "wh_xxxxx",
    "eventId": "whevt_xxxxx",
    "sequenceNumber": "10000000000",
    "createdAt": "2026-01-01T00:00:00.000Z"
  }
}
```

### Pub/Sub Messages

Published as a batch containing all transfer events from a webhook, with message attributes:

- `webhook_id`: Alchemy webhook ID
- `event_id`: Alchemy event ID
- `network`: Network name (e.g., ETH_MAINNET)
- `count`: Number of transfers in the batch

Published synchronously before returning response. If publishing fails, webhook will return 500 and Alchemy will retry.

### Firestore Documents

Stored in `alchemy_stream` collection with document ID format: `{txHash}-{logIndex}` to ensure idempotency.

**Transaction Guarantees:**

- Atomic writes using Firestore transactions
- Automatic batch splitting for large datasets (max 500 documents per transaction)
- All-or-nothing guarantee per batch - safe for retries

## Project Structure

```
alchemy-webhook/
├── function.go       # Cloud Function entry point with signature verification
├── parser.go         # ERC20 Transfer event parser using go-ethereum ABI decoder
├── pubsub.go         # Pub/Sub publisher with batch publishing
├── firestore.go      # Firestore storage with transactional writes
├── cloudbuild.yaml   # Cloud Build configuration
├── go.mod            # Go module dependencies
└── .env.example      # Environment variable template
```

## Implementation Details

### Security

- HMAC-SHA256 signature verification for all incoming webhooks
- Signature checked before any processing occurs
- Invalid signatures return 403 Forbidden

### Error Handling

- Failed signature validation: Returns 403 (no retry)
- JSON parsing errors: Returns 400 (no retry)
- Pub/Sub failures: Returns 500 (Alchemy retries)
- Firestore write failures: Returns 500 (transaction rolled back, Alchemy retries)

### Performance

- Synchronous Pub/Sub publishing for reliable delivery
- Synchronous Firestore writes for data durability
- Pre-allocated slices for transfer parsing
- Batch processing for large datasets (500 documents per transaction)
- Both operations use request context for proper cancellation handling

## License

MIT
