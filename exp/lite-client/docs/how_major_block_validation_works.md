# How Major Block Validation Works in Accumulate (v3 API)

## 1. What is a MessageRecord?
A `MessageRecord[messaging.Message]` is a record that wraps a protocol message (such as a block anchor or transaction) and its associated metadata, including signatures. For major blocks, the message type is typically an `Anchor` or similar protocol message that represents a major block event.

## 2. What Does "Validating a Major Block" Mean?
Validating a major block means cryptographically verifying that the major block was signed by the required authority set, with enough valid signatures. In v3, this is done by validating the **message record** corresponding to the major block anchor message.

## 3. How Do You Get the MessageRecord for a Major Block?
- Query the v3 API for the anchor message (the block anchor) that represents the major block.
- The result is a `MessageRecord[messaging.Message]` whose `Message` field is an `*protocol.Anchor`.

## 4. How Do You Validate It?
- Pass the `MessageRecord` (with signatures) to the validator.
- The validator checks the signatures against the authority set for that block (it knows how to look up the correct authority set based on block index/time).

## 5. Practical Steps
1. **Query for the anchor message** (the major block anchor) using its hash or URL.
2. **Receive a `MessageRecord`** for that anchor.
3. **Call your `ValidateMessageRecord` function** (which wraps it in an envelope and calls the v3 API’s `Validate`).
4. **If it returns true, the major block is valid** (signed by enough authorities).

### Example (Go Pseudocode)
```go
// 1. Get the anchor message record for the major block
msgRec, err := QueryMessageRecord(ctx, querier, anchorUrl) // anchorUrl is the URL of the anchor message for the major block
if err != nil {
    // handle error
}

// 2. Validate the message record
valid, err := ValidateMessageRecord(ctx, validator, msgRec)
if err != nil {
    // handle error
}
if !valid {
    // signatures are invalid or insufficient
}
```

## 6. How Do You Get the Anchor URL?
Usually, you get the anchor URL from the major block index or from a chain query (e.g., by querying the anchor chain for the nth major block). The anchor message is the canonical representation of the major block in Accumulate.

## 7. Summary
- **You do NOT validate a "MajorBlock" struct directly.**
- You **validate the anchor message** (which is a protocol message, wrapped in a MessageRecord, and signed by authorities).
- The v3 API validator knows how to check signatures and authority sets for the anchor message, which is what constitutes a valid major block.
