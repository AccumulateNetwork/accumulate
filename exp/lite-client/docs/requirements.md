# Accumulate Lite Client – Functional Requirements

## Overview

The Accumulate Lite Client allows users to monitor specific ADIs (Accumulate Digital Identities) and retrieve validated, up-to-date account state and data without running a full node. It achieves this by caching validated proofs and data, constructing and verifying Merkle receipts when needed, and pruning unused information.

---

## 1. User-Specified ADI Management

- **1.1** The user provides a list of ADI URLs to monitor.
- **1.2** The Lite Client must allow dynamic updates to the list:
  - Add new ADIs.
  - Remove specific ADIs.
  - Remove specific accounts under a given ADI.

---

## 2. Cache Lookup and Freshness Check

- **2.1** On user request, the client checks its internal cache for account data associated with the ADI.
- **2.2** If cached data exists, the client:
  - Verifies freshness via stored metadata (e.g., root hash, block height, or timestamp).
- **2.3** If data is fresh:
  - Return the cached **account data only** to the user.
  - Skip any receipt revalidation or remote API calls.

---

## 3. Receipt Construction and Verification (on Miss or Stale Cache)

If data is missing or outdated:

### 3.1 Construct Receipt

- **3.1.1** Traverse the account's main chain to generate a receipt.
- **3.1.2** Follow Merkle path:
  - From account state entry,
  - To the BVN anchor,
  - To the DN anchor.
- **3.1.3** Collect:
  - Merkle siblings,
  - Merkle root,
  - State entry,
  - Chain metadata.

### 3.2 Verify Receipt

- **3.2.1** Validate the Merkle proof locally.
- **3.2.2** Confirm root hash inclusion in a valid, signed block anchor:
  - Use operator key book for signature verification.
  - Validator set may be cached or fetched.

---

## 4. Account Data Retrieval and Processing

- **4.1** After proof verification, fetch current account data via:
  - v2 or v3 Accumulate API.
- **4.2** Identify the account type:
  - Token, staking, identity, key page, etc.
- **4.3** Parse relevant fields accordingly.
- **4.4** Format and return the final account data to the user.

---

## 5. Caching

- **5.1** Store:
  - The validated receipt,
  - The enriched account data.
- **5.2** Cache is indexed by:
  - ADI URL,
  - Account URL (e.g., `acc://adi.acme/token`).
- **5.3** Store metadata:
  - Last verified root,
  - Block height,
  - Timestamp of last update.

---

## 6. Pruning and Cache Invalidation

- **6.1** User can request to prune:
  - Entire ADIs,
  - Specific accounts under an ADI.
- **6.2** The Lite Client will:
  - Remove associated cache entries,
  - Optionally log the operation for audit/debug purposes.

---

## 7. Future Enhancements (Optional)

- [ ] Support startup warm-up: preload cache with known ADIs.
- [ ] TTL-based automatic cache expiry.
- [ ] Logging/metrics: hits, misses, proof failures, update delays.
- [ ] Pluggable storage backend: LevelDB, file, or in-memory.
- [ ] Support for multi-account batch queries.

---

## Notes

- All validation steps should follow the security guarantees of the full node:
  - Merkle inclusion proofs,
  - Signature threshold enforcement,
  - Chain-of-trust anchoring from account → BVN → DN.
- If proof or signature verification fails at any stage, the client must return a clear error to the user and avoid caching incomplete data.

