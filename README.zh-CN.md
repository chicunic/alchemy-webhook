# Alchemy Webhook

Alchemy webhook 工具，用于接收和处理 ERC20 Token Transfer 事件。

关于 Alchemy Notify API 的更多信息，请参阅[官方文档](https://www.alchemy.com/docs/reference/notify-api-quickstart)。

## 功能特性

1. **Webhook 签名验证** - 使用 HMAC-SHA256 安全签名验证
2. **Cloud Function** - 接收 webhook 事件并处理
3. **Pub/Sub 集成** - 可靠消息发布，自动重试
4. **Firestore 存储** - 事务性持久化，自动批处理（每个事务最多 500 个文档）

## GraphQL 查询

本项目专门监听 ERC20 Transfer 事件，使用以下 GraphQL 查询：

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

**说明：**

- `addresses` - 要监听的 ERC20 合约地址
- `topics[0]` - ERC20 Transfer 事件签名 `0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef`
  - 这是 `Transfer(address,address,uint256)` 事件的 Keccak-256 哈希值
  - 所有符合 ERC20 标准的 Token 合约都使用相同的事件签名
  - `topics[1]` 为 `from` 地址，`topics[2]` 为 `to` 地址，`data` 为转账数量

## Webhook 事件示例

接收到的事件格式：

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

## 快速开始

### 部署 Cloud Function

使用 Cloud Build 部署：

```bash
gcloud beta builds submit --config cloudbuild.yaml
```

## 环境变量

```bash
# 必需
ALCHEMY_SIGNING_KEY=your_signing_key_here

# 可选
ENABLE_PUBSUB=true
ALCHEMY_PUBSUB_TOPIC=your-topic-id
ENABLE_FIRESTORE=true
```

## 数据处理

每个 Transfer Event 会被处理成一个独立的文档，包含区块信息、交易信息和转账详情：

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

### Pub/Sub 消息

以批处理方式发布，包含来自一个 webhook 的所有转账事件，消息属性包括：

- `webhook_id`: Alchemy webhook ID
- `event_id`: Alchemy 事件 ID
- `network`: 网络名称（如 ETH_MAINNET）
- `count`: 批次中的转账数量

同步发布，在返回响应前完成。如果发布失败，webhook 返回 500，Alchemy 会重试。

### Firestore 文档

存储在 `alchemy_stream` 集合，文档 ID 格式：`{txHash}-{logIndex}`，确保幂等性。

**事务保证：**

- 使用 Firestore 事务进行原子写入
- 大数据集自动批量拆分（每个事务最多 500 个文档）
- 每个批次全部成功或全部失败 - 可安全重试

## 项目结构

```text
alchemy-webhook/
├── function.go       # Cloud Function 入口，包含签名验证
├── parser.go         # ERC20 Transfer 事件解析，使用 go-ethereum ABI 解码器
├── pubsub.go         # Pub/Sub 发布器，支持批量发布
├── firestore.go      # Firestore 存储，使用事务写入
├── cloudbuild.yaml   # Cloud Build 配置
├── go.mod            # Go 模块依赖
└── .env.example      # 环境变量模板
```

## 实现细节

### 安全性

- 对所有传入 webhook 进行 HMAC-SHA256 签名验证
- 在任何处理之前检查签名
- 无效签名返回 403 Forbidden

### 错误处理

- 签名验证失败：返回 403（不重试）
- JSON 解析错误：返回 400（不重试）
- Pub/Sub 失败：返回 500（Alchemy 重试）
- Firestore 写入失败：返回 500（事务回滚，Alchemy 重试）

### 性能优化

- 同步 Pub/Sub 发布，保证可靠传递
- 同步 Firestore 写入，保证数据持久性
- Transfer 解析使用预分配切片
- 大数据集批处理（每个事务 500 个文档）
- 两个操作都使用请求 context，正确处理取消

## 许可证

MIT
