# Accumulate Envelope Construction Guide

## Correction: Envelopes ARE Just JSON!

**Important:** You can absolutely create batch envelopes manually. Envelopes are standard JSON structures that can be constructed and submitted via API.

## Envelope Structure

An Accumulate envelope is a JSON object containing transactions and signatures:

```json
{
  "transaction": [
    {
      "header": {
        "principal": "acc://alice.acme/tokens",
        "initiator": "sha256_hash_of_first_signature"
      },
      "body": {
        "type": "sendTokens",
        "to": [
          {
            "url": "acc://bob.acme/tokens",
            "amount": "1000000000"
          }
        ]
      }
    },
    {
      "header": {
        "principal": "acc://alice.acme/tokens",
        "initiator": "sha256_hash_of_first_signature"
      },
      "body": {
        "type": "sendTokens",
        "to": [
          {
            "url": "acc://charlie.acme/tokens",
            "amount": "2000000000"
          }
        ]
      }
    }
  ],
  "signatures": [
    {
      "type": "ed25519",
      "publicKey": "0123456789abcdef...",
      "signature": "fedcba9876543210...",
      "signer": "acc://alice.acme/book/1",
      "signerVersion": 1,
      "timestamp": 1697123456000000000
    }
  ]
}
```

## Creating Batch Envelopes

### Step 1: Build Transaction Array

Each transaction in the array has:
- **header**: Contains principal and initiator
- **body**: Transaction-specific payload

```json
{
  "transaction": [
    // Transaction 1
    {
      "header": {
        "principal": "acc://alice.acme/tokens"
      },
      "body": {
        "type": "sendTokens",
        "to": [{
          "url": "acc://bob.acme/tokens",
          "amount": "1000000000"
        }]
      }
    },
    // Transaction 2
    {
      "header": {
        "principal": "acc://alice.acme/data"
      },
      "body": {
        "type": "writeData",
        "entry": {
          "data": ["SGVsbG8gV29ybGQ="]  // Base64: "Hello World"
        }
      }
    }
  ]
}
```

### Step 2: Add Signatures

Sign each transaction and add signatures to the envelope:

```json
{
  "signatures": [
    {
      "type": "ed25519",
      "publicKey": "public_key_hex",
      "signature": "signature_hex",
      "signer": "acc://alice.acme/book/1",
      "signerVersion": 1,
      "timestamp": 1697123456000000000
    }
  ]
}
```

### Step 3: Submit via API

Submit the complete envelope using the V3 API:

**HTTP:**
```bash
curl -X POST https://mainnet.accumulatenetwork.io/v3 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "submit",
    "params": {
      "envelope": {
        "transaction": [...],
        "signatures": [...]
      }
    }
  }'
```

**Via MCP (if tool exists):**
```json
{
  "tool": "accumulate_submit_transaction",
  "params": {
    "envelope": {
      "transaction": [...],
      "signatures": [...]
    }
  }
}
```

## Common Batch Patterns

### Pattern 1: Multiple Payments

Send tokens to multiple recipients in one envelope:

```json
{
  "transaction": [
    {
      "header": { "principal": "acc://company.acme/payroll" },
      "body": {
        "type": "sendTokens",
        "to": [{ "url": "acc://employee1.acme/tokens", "amount": "5000000000" }]
      }
    },
    {
      "header": { "principal": "acc://company.acme/payroll" },
      "body": {
        "type": "sendTokens",
        "to": [{ "url": "acc://employee2.acme/tokens", "amount": "6000000000" }]
      }
    },
    {
      "header": { "principal": "acc://company.acme/payroll" },
      "body": {
        "type": "sendTokens",
        "to": [{ "url": "acc://employee3.acme/tokens", "amount": "5500000000" }]
      }
    }
  ],
  "signatures": [...]
}
```

### Pattern 2: Account Setup

Create multiple accounts in one batch:

```json
{
  "transaction": [
    {
      "header": { "principal": "acc://alice.acme" },
      "body": {
        "type": "createTokenAccount",
        "url": "acc://alice.acme/tokens",
        "tokenUrl": "acc://ACME",
        "authorities": ["acc://alice.acme/book"]
      }
    },
    {
      "header": { "principal": "acc://alice.acme" },
      "body": {
        "type": "createDataAccount",
        "url": "acc://alice.acme/data",
        "authorities": ["acc://alice.acme/book"]
      }
    }
  ],
  "signatures": [...]
}
```

### Pattern 3: Atomic Operations

Update account and write audit log atomically:

```json
{
  "transaction": [
    {
      "header": { "principal": "acc://alice.acme/tokens" },
      "body": {
        "type": "sendTokens",
        "to": [{ "url": "acc://bob.acme/tokens", "amount": "1000000000" }]
      }
    },
    {
      "header": { "principal": "acc://alice.acme/audit" },
      "body": {
        "type": "writeData",
        "entry": {
          "data": ["UGFpZCBCb2IgMTAgQUNNRQ=="]  // "Paid Bob 10 ACME"
        }
      }
    }
  ],
  "signatures": [...]
}
```

## Transaction Types Reference

Common transaction types for envelopes:

| Type | Purpose | Body Fields |
|------|---------|-------------|
| `sendTokens` | Send tokens | `to` (array of recipients) |
| `createIdentity` | Create ADI | `url`, `keyHash`, `authorities` |
| `createTokenAccount` | Create token account | `url`, `tokenUrl`, `authorities` |
| `createDataAccount` | Create data account | `url`, `authorities` |
| `createKeyPage` | Create key page | `url`, `keys` |
| `createKeyBook` | Create key book | `url`, `publicKeyHash` |
| `writeData` | Write data entry | `entry` with `data` array |
| `updateKeyPage` | Update keys | `operation` (add/remove/update) |
| `updateAccountAuth` | Update authorities | `operations` array |
| `issueTokens` | Issue new tokens | `recipient`, `amount` |
| `burnTokens` | Burn tokens | `amount` |
| `addCredits` | Add credits | `recipient`, `amount` |

See [protocol/types.yml](../../protocol/types.yml) for complete list of 60+ transaction types.

## Signature Requirements

### Single Signer (Simple Case)

If all transactions use same authority:

```json
{
  "signatures": [
    {
      "type": "ed25519",
      "publicKey": "...",
      "signature": "...",
      "signer": "acc://alice.acme/book/1",
      "signerVersion": 1,
      "timestamp": 1697123456000000000
    }
  ]
}
```

### Multiple Signers

If transactions require different authorities:

```json
{
  "signatures": [
    {
      "signer": "acc://alice.acme/book/1",
      "type": "ed25519",
      "publicKey": "...",
      "signature": "...",
      "signerVersion": 1,
      "timestamp": 1697123456000000000
    },
    {
      "signer": "acc://bob.acme/book/1",
      "type": "ed25519",
      "publicKey": "...",
      "signature": "...",
      "signerVersion": 1,
      "timestamp": 1697123456000000000
    }
  ]
}
```

## Benefits of Batch Envelopes

1. **Gas Efficiency**: Single submission fee for multiple transactions
2. **Atomicity**: All transactions execute together (or fail together)
3. **Reduced Latency**: One network round-trip instead of multiple
4. **Logical Grouping**: Related operations bundled

## Limitations

- **Max Envelope Size**: Network-dependent (test with 10-20 transactions)
- **All-or-Nothing**: If one transaction fails, entire envelope may fail
- **Same Block**: All transactions processed in same block

## API Endpoints

### V3 Submit

**JSON-RPC:**
```
POST /v3
Method: "submit"
Params: { "envelope": {...} }
```

**REST:**
```
POST /v3/submit
Body: { "envelope": {...} }
```

### V2 Execute (Legacy)

```
POST /v2
Method: "execute"
Params: { "payload": [...], "signature": {...} }
```

## MCP Tool Support

### Current Status

**mcp-accumulate implementation:**
- ❌ No standalone `accumulate_submit_transaction` tool (yet)
- ✅ All transaction tools build envelopes internally
- ✅ Client code supports `SubmitEnvelope()` method
- ✅ Tests validate batch operations

**Recommendation:**
Add generic submit tool:

```json
{
  "name": "accumulate_submit_envelope",
  "description": "Submit a pre-constructed envelope (single or batch)",
  "inputSchema": {
    "type": "object",
    "properties": {
      "envelope": {
        "type": "object",
        "description": "Complete envelope with transactions and signatures",
        "properties": {
          "transaction": {
            "type": "array",
            "description": "Array of transactions"
          },
          "signatures": {
            "type": "array",
            "description": "Array of signatures"
          }
        }
      },
      "wait": {
        "type": "boolean",
        "description": "Wait for transaction completion"
      }
    }
  }
}
```

## Examples from Tests

### Batch of 3 Token Sends

From `integration_envelopes_test.go`:

```go
// Create 3 transactions
txns := []*protocol.Transaction{txn1, txn2, txn3}
sigs := []protocol.Signature{sig1, sig2, sig3}

// Submit as batch
hashes, err := client.SubmitEnvelope(ctx, txns, sigs)
// Returns 3 transaction hashes
```

Equivalent JSON envelope:
```json
{
  "transaction": [
    { "header": {...}, "body": {"type": "sendTokens", ...} },
    { "header": {...}, "body": {"type": "sendTokens", ...} },
    { "header": {...}, "body": {"type": "sendTokens", ...} }
  ],
  "signatures": [
    { "type": "ed25519", "signer": "acc://alice.acme/book/1", ... }
  ]
}
```

### Large Scale (10 Transactions)

Tested with 10 transactions in single envelope - all succeeded.

## AI Assistant Usage

As an AI assistant, you can:

1. **Construct JSON envelopes** - Build the transaction array
2. **Request signatures** - Ask user to sign with private keys
3. **Submit via API** - Use HTTP or MCP tool (when available)

Example workflow:
```
AI: "I'll create a batch envelope with 3 payments..."
AI: [constructs JSON with 3 sendTokens transactions]
AI: "Please sign with your key at acc://alice.acme/book/1"
User: [provides signature]
AI: [adds signature to envelope]
AI: [submits via API]
AI: "All 3 payments submitted! Transaction hashes: [...]"
```

## Conclusion

**YES - You can create batch envelopes manually!**

- Envelopes are standard JSON
- Structure is well-defined
- Can be submitted via V3 API
- AI assistants can construct them
- MCP tools could/should expose this

The gap is not in the protocol or API - it's in the MCP tooling not exposing a generic submit endpoint.

## References

- Envelope tests: `mcp-accumulate/integration_envelopes_test.go`
- Client code: `mcp-accumulate/client/client.go` (SubmitEnvelope method)
- Protocol types: `accumulate/pkg/types/messaging/types.go`
- V3 API: `accumulate/pkg/api/v3/`

## Version History

- **v1.0** (2025-10-20): Initial guide
  - Corrected misconception about envelope creation
  - Documented JSON structure
  - Added batch patterns
  - Included examples from tests
