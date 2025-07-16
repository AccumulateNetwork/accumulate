# Testing the Lite Client Against a Live Network

## 1. Introduction

This document provides a guide for testing the Accumulate lite client against a live network like the Kermit testnet. Unit tests with mocked data are essential, but they cannot fully replicate the behavior and potential edge cases of a live, dynamic blockchain. Testing against a real network is the only way to ensure your validation logic is robust and correct.

**Primary Goal:** To verify that the lite client can correctly sync from genesis, validate major block signatures, track authority changes, and prove the integrity of the chain using live data.

## 2. Prerequisites

- **Kermit Testnet Endpoint:** You need a reliable RPC endpoint for the Kermit testnet. The lite client should be configurable to point to this endpoint.
- **`accumulate` CLI:** The command-line interface is an invaluable tool for cross-referencing data and manually querying the network to verify what your client is seeing.

## 3. High-Level Testing Strategy

The core of the testing process is to perform the validation described in Phase 2 of `lite_client.md`, but using live network data instead of mocks.

### Step 1: Configure Your Client for Kermit

Ensure your lite client's RPC provider is pointing to a Kermit testnet endpoint. This is the foundational step for all subsequent tests.

### Step 2: Fetching the Genesis Block and Initial Authorities

Your validation must start from a trusted root: the genesis block.

1.  **Query Genesis:** Programmatically fetch the block at height 0.
2.  **Extract Initial Authorities:** The genesis block defines the initial set of validator authorities. You must parse this information and store it as your starting trust root.
    - The authorities are typically stored in a system account like `acc://dn.acme/validators`.
3.  **Cross-Reference with CLI:** Use the `accumulate` CLI to manually inspect the genesis block and the validator set to ensure your client is parsing this critical information correctly.

### Step 3: Fetching Major Blocks and Finding Anchors (v3 API)

Once you have the genesis state, your client must iterate through major blocks sequentially using the v3 API, which provides the necessary anchor and signature data.

1.  **Use `QueryMajorBlocks` (v3 API):** Start from block 1 and fetch major blocks. The v3 response (`MajorBlockRecord`) is comprehensive.
2.  **Locate the Anchor:** The signatures you need to validate are not on the major block itself, but on a `BlockAnchor` message. This message is typically found within the minor blocks included in the major block record.
    - Iterate through `MajorBlockRecord.MinorBlocks`.
    - Inside each minor block, iterate through `MinorBlock.Entries`.
    - Look for an entry that is a `MessageRecord` containing a `BlockAnchor` message and an associated `SignatureSetRecord`.
3.  **Extract Data:** Once found, extract the `BlockAnchor` message and the `SignatureSetRecord` containing the validator signatures.

### Step 4: Validating the Block Anchor Signature

This is the core of Phase 2 validation.

1.  **Hash the Block Anchor:** The signature is for the `BlockAnchor` message, **not** the entire `MajorBlockRecord`. You must compute the hash of the `BlockAnchor` message (`BlockAnchor.Hash()`).
2.  **Identify the Correct Authority Set:** For the given major block's height, retrieve the authority set that was active at that time. For early blocks, this will be the genesis authority set.
3.  **Perform Cryptographic Verification:** Use the `Ed25519SignatureVerifier` (or equivalent) to verify the signatures from the `SignatureSetRecord` against the hash of the `BlockAnchor` and the public keys of the active authority set.
4.  **Check Threshold:** Ensure the number of valid signatures meets or exceeds the required threshold.

### Step 5: Handling Authority Changes

Your client must detect and securely handle updates to the validator set to remain in sync with the network's chain of trust.

**Conceptual Flow:**
An authority change is a transaction on a system account (`acc://dn.acme/validators`) that must be signed by the previous set of authorities.

**Practical Implementation (Pseudocode):**
```go
// As you process blocks, check for authority updates
func checkForAuthorityUpdate(block) {
    for _, tx := range block.Transactions {
        // 1. Detect: Is this a transaction on the DN's validator page?
        if tx.Header.Principal.Equal(url.MustParse("acc://dn.acme/validators")) {
            // 2. Identify: Is it an authority-changing operation (e.g., UpdateKeyPage)?
            if _, ok := tx.Body.(*protocol.UpdateKeyPage); ok {
                // 3. Validate: Verify the transaction's signatures using the *current* authority set
                err := CurrentAuthoritySet.Validate(tx.Signatures, tx.Hash())
                if err != nil {
                    // This is a critical failure - an invalid authority change!
                    panic("Invalid authority change detected!")
                }

                // 4. Apply: Extract the new public keys from the transaction body
                newKeys := extractNewKeysFromTx(tx.Body)
                NextAuthoritySet = NewAuthoritySet(newKeys)

                // The new set becomes active for all blocks *after* this one
                log.Printf("Authority set will update at block %d", block.Height+1)
            }
        }
    }
}
```

**A Note on `GenesisAuthorityProvider`:**
The `GenesisAuthorityProvider` is a simple, static implementation that only knows about the initial genesis authorities. It is sufficient for validating the first few blocks but **cannot handle authority changes**. For a fully compliant lite client, you must implement a dynamic `AuthorityProvider` that tracks and applies validated authority changes over time.

## 4. Useful Tools and Commands

Use the `accumulate` CLI to double-check the work of your lite client.

- **Get Block Info:**
  ```bash
  accumulate get block <partition> <height>
  ```
- **Query a specific transaction by hash:**
  ```bash
  accumulate tx get <txid>
  ```
- **Inspect an account:**
  ```bash
  accumulate account get <account_url>
  ```

## 5. Common Pitfalls

- **API Version Mismatch:** Ensure you are exclusively using the v3 API for major block queries to get the rich data you need.
- **Incorrect Hash Target:** Always double-check that you are hashing the `BlockAnchor` for signature validation, not the major block or another message.
- **Static Authorities:** Failing to implement dynamic authority tracking will cause validation to fail as soon as the first authority change occurs.
- **Network Errors:** Implement robust retry and backoff mechanisms for API calls to handle transient network issues.

## ✅ Checklist for Phase 2 Success

- [ ] Genesis authority set is fetched and cached correctly.
- [ ] Major blocks are fetched sequentially from genesis using the **v3 API**.
- [ ] `BlockAnchor` and `SignatureSetRecord` are correctly extracted from minor block entries within the `MajorBlockRecord`.
- [ ] The hash of the `BlockAnchor` message is computed and verified against the signatures.
- [ ] Signatures are successfully verified against the public keys of the correct authority set.
- [ ] Authority changes on `acc://dn.acme/validators` are detected, validated with the previous set, and applied for future blocks.
- [ ] The implementation does not rely on mocks, hardcoded values, or shortcuts when running in a live test environment.
