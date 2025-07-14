# Accumulate API Learnings for Lite Client

This document outlines the key learnings and patterns for interacting with the Accumulate v2 and v3 APIs, based on the development of the lite client.

## 1. API Overview: v2 vs. v3

The lite client utilizes a hybrid approach, leveraging both the v2 and v3 APIs. Each API version serves a distinct purpose.

-   **V2 API (`pkg/client/api/v2`)**: Primarily used for fetching account states and cryptographic proofs (Merkle receipts). It is well-suited for retrieving data that doesn't require signature-level detail.
-   **V3 API (`pkg/api/v3`)**: Used for deep inspection of chain data, especially for validating signatures. It provides access to raw message records, signature sets, and a `Validator` interface that simplifies signature verification against the correct authority set.

## 2. V2 API In-Depth

### Client Initialization

A v2 client is created by providing the node's server URL.

```go
import client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"

v2Client, err := client.New("https://kermit.accumulatenetwork.io")
```

### Fetching BPT Root Hash

The BPT root hash for the DN or a BVN can be fetched from the `/status` endpoint.

-   **File**: `exp/lite-client/bpt.go`
-   **Function**: `FetchBPTRootHash(ctx, cl, partition)`
-   **Logic**: Calls `cl.Status(ctx)` and returns `status.DnBptHash` or `status.BvnBptHash`.

### Fetching Proofs

Cryptographic proofs (receipts) for an account's state are fetched using the generic `RequestAPIv2` method targeting the `query` endpoint.

-   **File**: `exp/lite-client/proof.go`
-   **Function**: `FetchProof(api, ctx, account)`
-   **Logic**:
    1.  Creates a `client.GeneralQuery` with the account URL.
    2.  Calls `api.RequestAPIv2(ctx, "query", req, &resp)`.
    3.  The response (`client.ChainQueryResponse`) contains a `Receipt` field, which holds the Merkle proof (`resp.Receipt.Proof`).

### Querying Major Blocks (v2)

The v2 API can query major blocks, but the response **lacks signature data**.

-   **File**: `exp/lite-client/blocks/block_major.go`
-   **Function**: `QueryMajorBlocksV2(...)`
-   **Logic**: Uses `cl.QueryMajorBlocks(ctx, query)`.
-   **Limitation**: The `client.MajorQueryResponse` contains the block index and time but no signatures, making it unsuitable for Phase 2 validation.

## 3. V3 API In-Depth

### Client Initialization

A v3 client is a JSON-RPC client, also initialized with a server URL.

```go
import "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"

v3Client := jsonrpc.NewClient("https://kermit.accumulatenetwork.io/v3")
```

### The `Querier` and `Validator` Interfaces

The v3 API is structured around interfaces:
-   `api.Querier`: Provides a generic `Query` method.
-   `api.Querier2`: A helper struct that wraps `Querier` and provides specific, type-safe query methods (e.g., `QueryMajorBlocks`, `QueryMessage`).
-   `api.Validator`: Provides a `Validate` method to verify signatures on an envelope.

### Querying Major Blocks (v3)

Querying major blocks with full details, including signatures, is done via the v3 API.

-   **File**: `exp/lite-client/blocks/block_major.go`
-   **Logic**:
    1.  Use `api.Querier2` to call `QueryMajorBlocks`.
    2.  The query parameter is `*api.BlockQuery`, which contains a `MajorRange` (`*api.RangeOptions`) to specify start and count.
    3.  The response is `*api.RecordRange[*api.MajorBlockRecord]`.

### `MajorBlockRecord` Structure

The `*api.MajorBlockRecord` is a rich object. It contains a `MinorBlocks` field, which is a `RecordRange` of minor blocks. Each minor block contains `Entries`, which can be:
-   `*api.MessageRecord[messaging.Message]`: Often a `*messaging.BlockAnchor`.
-   `*api.SignatureSetRecord`: Contains the signatures for the anchor.

### Signature Validation

This is the core strength of the v3 API for the lite client.

-   **File**: `exp/lite-client/blocks/validate.go`
-   **Function**: `ValidateMajorBlock(ctx, validator, block)`
-   **Logic**:
    1.  Iterate through the minor blocks within a `MajorBlockRecord` to find the `BlockAnchor` message and the corresponding `SignatureSetRecord`.
    2.  Extract the signatures from the signature set.
    3.  Construct an `envelope` containing the anchor message and its signatures.
    4.  Call `validator.Validate(ctx, envelope, ...)` to perform the validation. The validator automatically handles fetching the correct authority set for the given block height.

## 4. Current Hybrid Strategy

The lite client's `RetrieveAccountStates` function orchestrates a two-phase process:

**Phase 1: Proof Validation (v2 API)**
1.  `FetchBPTRootHash` is called to get the latest BPT root from the network's `/status` endpoint.
2.  For each account, `ValidateAndCacheProof` is called.
3.  `FetchProof` uses the v2 API to get a receipt for the account.
4.  `VerifyProof` validates the receipt against the fetched BPT root hash.

**Phase 2: Signature Validation (v3 API)**
1.  The client iterates through all major blocks from genesis to the latest.
2.  `blocks.QueryMajorBlocksV3` is used to fetch each `MajorBlockRecord`.
3.  `blocks.ValidateMajorBlock` is called for each block. This function uses the v3 `Validator` to verify the signatures on the block's anchor, ensuring a continuous chain of trust from genesis.

This hybrid model uses the most effective API for each task: v2 for its simple, direct access to proofs, and v3 for its powerful, detailed signature validation capabilities.
