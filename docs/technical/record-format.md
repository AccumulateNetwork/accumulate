# Accumulate Snapshot Record Format

This document details the exact format of records stored in Accumulate snapshots, including how to decode various record types, account structures, chains, and transaction ordering.

## Record Structure Overview

All records in an Accumulate snapshot follow a common structure:

```
Record {
    Key    []byte  // Variable length key (length-prefixed hierarchical path)
    Value  []byte  // Variable length value (type-specific encoded data)
}
```

## Record Key Format

Record keys are hierarchical paths encoded as sequences of length-prefixed strings. Each component is prefixed with a single byte indicating its length.

### Key Structure

```
Key {
    Component1Length uint8
    Component1       []byte  // Length specified by Component1Length
    Component2Length uint8
    Component2       []byte  // Length specified by Component2Length
    ...
}
```

### Common Key Prefixes

Records are organized by their key prefixes:

1. `Account/{url}` - Account data
2. `Account/{url}/{chain-type}` - Chain data for an account
3. `Message/{hash}` - Message data
4. `Transaction/{hash}` - Transaction data

## Account Records

Account records contain the serialized state of Accumulate accounts.

### Account Key Format

```
Key = "Account" + URL
```

Where:
- `URL` is the full Accumulate URL of the account (e.g., "acc://example/token")

### Account Value Format

The value is a serialized protocol.Account message using protocol buffers. The specific account type can be determined by examining the `Type` field.

```
Value = protobuf(protocol.Account)
```

Account types include:
- `protocol.LiteIdentity`
- `protocol.ADI`
- `protocol.TokenAccount`
- `protocol.DataAccount`
- ...

### Decoding Account Records

1. Parse the key to extract the URL
2. Deserialize the value as a protocol.Account
3. Use type assertion or switch on the Type field to access type-specific fields

## Chain Records

Chain records store the state of chains associated with accounts.

### Chain Key Format

```
Key = "Account" + URL + ChainType
```

Where:
- `URL` is the account URL
- `ChainType` is the type of chain (e.g., "main", "pending", "scratch", etc.)

### Chain Value Format

```
Value = protobuf(protocol.Chain)
```

The Chain structure contains:
- Chain type
- Chain height
- Root anchor hash
- Recent anchor hashes

## Transaction and Message Records

### Message Key Format

```
Key = "Message" + MessageHash
```

Where:
- `MessageHash` is the 32-byte hash of the message

### Message Value Format

```
Value = protobuf(protocol.Message)
```

### Transaction Key Format

```
Key = "Transaction" + TransactionHash
```

Where:
- `TransactionHash` is the 32-byte hash of the transaction

### Transaction Value Format

```
Value = protobuf(protocol.Transaction)
```

## Chain Entry Organization

Chain entries (transactions and messages) are organized in chronological order within their respective chains.

### Transaction Ordering in Chains

Transactions within a chain are ordered by:
1. Block height (ascending)
2. Transaction index within the block (ascending)

This ordering is maintained by the chain's internal data structure and is preserved when records are written to a snapshot.

### Message Ordering

Messages are ordered by their hash values in ascending order when written to a snapshot. This ensures:
1. Deterministic processing
2. Efficient deduplication
3. Consistent ordering across different snapshots

## Special Record Types

### System Records

System records store global state information:

```
Key = "System" + RecordType
Value = type-specific encoded data
```

Common system record types include:
- Network parameters
- Global ledger state
- Validator sets

### Index Records

Index records provide fast lookup capabilities:

```
Key = "Index" + IndexType + IndexKey
Value = target record key or other indexed data
```

## Binary Encoding Details

### Length Prefixing

All variable-length fields are prefixed with their length:
- For fields < 256 bytes: 1-byte length prefix
- For larger fields: varint-encoded length prefix

### Hash Encoding

All hashes are stored as raw 32-byte arrays without any encoding.

### URL Encoding

URLs are stored as UTF-8 encoded strings with proper length prefixing.

## Record Processing Order

When processing records from a snapshot:

1. Process account records first to establish the account hierarchy
2. Process chain records to establish chain state
3. Process transaction and message records to populate chains

This order ensures that all necessary structures are in place before dependent records are processed.
