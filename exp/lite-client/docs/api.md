
# Ultimate Guide to Querying the Accumulate Protocol Using v2 and v3 APIs

## Introduction
This guide is your comprehensive reference for querying data from the Accumulate blockchain using both the v2 and v3 APIs, including detailed notes on the v3 message client and lite client validation logic.

---

## 1. Overview: v2 vs v3

| Feature            | v2 API                                     | v3 API (JSON-RPC + Binary)               |
|--------------------|---------------------------------------------|------------------------------------------|
| Transport          | HTTP JSON-RPC                               | JSON-RPC + Binary (Message Transport)    |
| Use case           | Traditional client queries                  | Lite client, flexible routing, streaming |
| Signature support  | Limited                                     | Full support with validation flows       |

Use **v2** for high-level queries, **v3** for low-level validation, signature analysis, and proofs.

---

## 2. Setting Up Clients

### v2 API Setup (Go)
```go
import client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
cl, err := client.New("https://kermit.accumulatenetwork.io")
```

### v3 Message Client Setup (Go)
```go
import message "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"

cl := &message.Client{
    Transport: &message.RoutedTransport{
        Network: "Kermit",
        Dialer: message.NetDialer{},
        Router: myRouter,
    },
}
```

---

## 3. Querying with v2 API

### 3.1 Major Block Query (No Signature Support)
```go
query := &client.MajorBlocksQuery{
    Count: 2,
    Start: 0,
    Url: accurl.MustParse("acc://bvn0.acme"),
}
resp, err := cl.QueryMajorBlocks(ctx, query)
```

### 3.2 Parsing the Response
`MajorQueryResponse` includes:
```go
type MajorQueryResponse struct {
    MajorBlockIndex uint64
    MajorBlockTime  *time.Time
    MinorBlocks     []*MinorBlock
    LastBlockTime   *time.Time
}
```
**Limitation**: No block signature data is returned.

---

## 4. Querying with v3 API (Message Client)

### 4.1 Transport Interface
The `Transport` interface defines `RoundTrip` and `OpenStream`. The `RoutedTransport` routes messages based on partition and network.

### 4.2 Querying Message Records
```go
cl := &message.Client{Transport: yourTransport}
req := &api.GeneralQuery{Url: accurl.MustParse("acc://bvn0.acme")}
cl.Transport.RoundTrip(ctx, []Message{req}, callback)
```

### 4.3 Signature Extraction Types
```go
type MessageRecord[T messaging.Message] struct {
    Signatures *RecordRange[*SignatureSetRecord]
}

// To search for signatures by public key:
type PublicKeySearchQuery struct {
    PublicKey []byte
    Type      protocol.SignatureType
}
```

---

## 5. Lite Client Use Case: Validating Major Blocks

### Phase 2: Validate Signatures from Genesis to Present

#### Requirements:
- Retrieve each major block's root hash and signature
- Fetch authority keybooks (account authorities may change)
- Validate each major block's signature

### Challenges with v2:
- `MajorQueryResponse` does **not** include signatures
- Cannot validate block authenticity directly

### v3 Solution Path:
- Use `Client.Query()` to fetch messages for `BlockAnchor`
- Use `SignatureSetRecord` to locate and validate ED25519 or RCD1 signatures

---

## 6. Snapshots and State Proofs

For state validation:
- Compare BPT root hash from major block
- Construct receipts from BPT entries to prove account inclusion

### Snapshot Files:
- Include resolved account URLs and hash values
- Used for efficient auditing, healing, and restoration

---

## 7. Testing Strategies

### Sample Query Test (v2)
```go
func TestQueryMajorBlocksV2_Kermit(t *testing.T) {
    cl, _ := client.New("https://kermit.accumulatenetwork.io")
    blocks, _ := QueryMajorBlocksV2(ctx, cl, "acc://bvn0.acme", 0, 2)
    for i, b := range blocks {
        t.Logf("Block %d: Index=%v Time=%v", i, b.MajorBlockIndex, b.MajorBlockTime)
    }
}
```

### Sample Signature Validation (v3)
```go
func validateSignature(sig protocol.KeySignature, root []byte, pub []byte) bool {
    return protocol.VerifySignature(sig, root, pub) == nil
}
```

---

## 8. Querying Signature Records in v3

### Signature Retrieval by Account or Public Key
```go
q := api.Querier2{Transport: yourTransport}

// Query all signatures linked to a given account
resp, err := q.QueryMessageRecords(ctx, &api.MessageRecordsQuery{
    Recipient: accurl.MustParse("acc://bvn0.acme"),
    Limit:     1000,
})
```

### Extracting ED25519 Signatures
From the returned `RecordRange[*SignatureSetRecord]`:
```go
for _, sigSet := range resp.Signatures.Records {
    for _, sig := range sigSet.Signatures {
        ed, ok := sig.Signature.(*protocol.ED25519Signature)
        if ok {
            t.Logf("Signature from %x at Block %d", ed.PublicKey[:4], sigSet.SignatureBlockIndex)
        }
    }
}
```

### Public Key Search (Advanced)
```go
search := &api.PublicKeySearchQuery{
    PublicKey: []byte{...},
    Type:      protocol.SignatureTypeED25519,
}
resp, err := q.SearchForSignatureSets(ctx, search)
```

---

## 9. Getting Authority Key Pages

Use `QueryKeyBook` to trace authority delegation:
```go
keyBookResp, err := q.QueryAccount(ctx, accurl.MustParse("acc://my-id/book"))
book := keyBookResp.Account.(*protocol.KeyBook)
for _, page := range book.Pages {
    t.Logf("Key page: %s", page)
}
```

---

## 10. Walking Through a BlockAnchor

Retrieve the message and match the source partition or account:
```go
msgs, err := q.QueryMessages(ctx, &api.MessagesQuery{
    Cause: accurl.MustParse("acc://blockanchor"),
})
for _, msg := range msgs.Messages {
    anchor, ok := msg.Message.(*messaging.BlockAnchor)
    if ok {
        t.Logf("Block %d from %s with Root %x", anchor.MajorBlockIndex, anchor.Source, anchor.RootChainAnchor)
    }
}
```

---

## 11. Summary

| Feature                        | v2 API                     | v3 Message API             |
|-------------------------------|----------------------------|----------------------------|
| Major block timestamps        | ✅                        | ✅                        |
| Block signatures              | ❌ Not available         | ✅ With custom query     |
| Account query                 | ✅                        | ✅                        |
| Signature validation          | ❌ (indirect)            | ✅ Full support           |
| Custom receipt/merkle proofs  | ❌                       | ✅                        |

---

## 12. Next Steps
- Build helper functions in v3 to isolate signature records for major blocks
- Collect `BlockAnchor` messages to extract signatures
- Track authority changes using `KeyBook` + `KeyPage` queries over time
- Implement block index sequencing checks (phase 3)

For specific examples, dig into `pkg/api/v3/message/client.go` and look at test files like `TestValidator`, `TestQuerier`, and `TestSubmitter` for how to mock and validate these flows.
