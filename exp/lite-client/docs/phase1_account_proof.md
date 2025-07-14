# Phase 1: Account Proof Retrieval and Validation

## Objective
To independently fetch, verify, and cache cryptographic proofs of account states from the Accumulate network, ensuring the integrity of account data without trusting the node’s output blindly.

---

## Core Flow

### 1. Orchestration (`client.go`)
- The main entrypoint is `LiteClient.RetrieveAccountStates(ctx, accountUrls)`.
- This method calls `RetrieveAndValidateProof(ctx, accountUrls, c)` to handle Phase 1.

### 2. Proof Retrieval and Validation (`proof.go`)
- **Fetch BPT Root Hash:**  
  `FetchBPTRootHash(ctx, c.v2, "dn")` retrieves the BPT root hash from the node’s status endpoint for the specified partition (e.g., `"dn"` for Directory Network).
- **Fetch and Validate Proof for Each Account:**  
  For each account URL:
  - `ValidateAndCacheProof(client, ctx, account, rootHash)` is called.
  - This function:
    1. **Fetches the proof** from the node using `FetchProof`.
    2. **Verifies the proof** against the known root hash using `VerifyProof`.
    3. **Caches the verified proof** in the client’s in-memory cache for future use.

#### Key Functions
- `FetchProof(api *client.Client, ctx context.Context, account string) (*VerifiedAccount, error)`  
  Queries the v2 API for a cryptographic proof of the account state.
- `VerifyProof(receipt *merkle.Receipt, expectedRoot []byte) bool`  
  Verifies that the proof receipt matches the expected BPT root hash and is cryptographically valid.
- `ValidateAndCacheProof(client *LiteClient, ctx context.Context, account string, knownRoot []byte) error`  
  Orchestrates fetching, verifying, and caching the proof.

### 3. BPT Root Hash Handling (`bpt.go`)
- `FetchBPTRootHash(ctx, cl, partition)` fetches the BPT root hash for `"dn"` or `"bvn0.acme"` partitions using the node’s status.
- This root is used as the trust anchor for proof validation.

### 4. In-Memory Cache (`cache.go`)
- The `LiteClient` struct contains a `cache map[string]VerifiedAccount`.
- After a proof is validated, it is stored in this cache for quick retrieval and to avoid redundant network calls.
- The cache is in-memory only and is reset on each run, keeping the client stateless.

---

## Design Decisions
- **Statelessness:**  
  All proofs and states are held in memory for the duration of the run, with no persistent storage.
- **Trust Anchor:**  
  The BPT root hash is fetched from the node and used as the trust anchor for all proof validations.
- **Independent Verification:**  
  The client does not trust the node’s account state directly; it verifies cryptographic proofs against the root hash.
- **Error Handling:**  
  Any failure to fetch, verify, or cache a proof results in an explicit error and aborts the process for that account.
- **Extensibility:**  
  The architecture allows for easy extension to other types of proofs or additional validation logic.

---

## Example Flow
```go
// 1. Initialize LiteClient
client, _ := NewLiteClient(serverUrl)

// 2. Retrieve and validate proofs for a list of accounts
err := client.RetrieveAccountStates(ctx, []string{"acc://alice", "acc://bob"})
if err != nil {
    // Handle error (e.g., proof failed for one or more accounts)
}

// 3. Access the validated, cached proof
proof := client.cache["acc://alice"]
```

---

## Key Files and Responsibilities
| Component   | Responsibility                                     |
|-------------|----------------------------------------------------|
| client.go   | Orchestrates proof retrieval and validation        |
| proof.go    | Fetches, verifies, and caches cryptographic proofs |
| bpt.go      | Fetches BPT root hash (trust anchor)               |
| cache.go    | In-memory cache for proofs                         |

---
