# Accumulate Lite Client – Account API Documentation

## Overview

The **Account API** provides functionality to query and cache balance and transaction data for Accumulate token accounts. It is implemented on top of the `LiteClient` abstraction and uses the **Accumulate v2 API** for stable access to chain data. The system also incorporates **proof validation** using BPT root hashes and includes mechanisms for caching results.

This document explores the API’s design, internal file responsibilities, and the validation logic used with the Kermit testnet.

---

## Core Capabilities

The Account API supports:

* Retrieving token account balances
* Retrieving transactions (latest or all)
* Fetching both balance and transactions in one call
* Caching balances and transactions
* Validating Merkle proofs of account state
* Fetching and verifying BPT root hashes
* Connecting to the Kermit testnet ([https://testnet.accumulatenetwork.io](https://testnet.accumulatenetwork.io))

---

## High-Level Architecture

```
              +----------------+
              |  account_data  |
              |   _test.go     |
              +--------+-------+
                       |
                       v
+-------------------- Account API --------------------+
| get balance     ->  GetBalance()                    |
| get txs         ->  GetTransactions()               |
| get both        ->  GetBalanceAndTransactions()     |
| cache ops       ->  GetStored*, Store*              |
| full sync       ->  RetrieveAccountStates()         |
+-----------------------------------------------------+
                       |
          +------------+------------+
          |                         |
          v                         v
   BPT/Proof Tools           Accumulate v2/v3 API
   (bpt.go, proof.go)       (via `pkg/client/api/v2`)
```

---

## File Responsibilities

### 1. `liteclient.go`

Implements the `LiteClient` struct:

* Instantiates v2 and v3 clients from a given node URL.
* Provides high-level account orchestration:

  * `RetrieveAccountStates()` → calls:

    * `retrieveAndValidateProofs()` (calls BPT and proof logic)
    * `retrieveAccountData()` (uses v2 API to fetch balances & txs)
  * `PullAllTransactions()` → pulls all txs using v2
  * `StoreTransaction()`, `GetStoredTransactions()` → local cache

### 2. `api.go`

Implements the `TokenAccountAPI` interface:

* `GetBalance()` uses the v2 API to query balance (via `ChainQuery`)
* `GetTransactions()` uses `query-tx-history`
* `GetBalanceAndTransactions()` aggregates the two above
* These use `pkg/client/api/v2` to communicate with the full node—not the lite client API

### 3. `bpt.go`

Defines:

```go
FetchBPTRootHash(ctx, client, partition string) ([]byte, error)
```

This fetches the latest **BPT root hash** from the node status endpoint (used for proof validation). It is essential for verifying account Merkle receipts.

### 4. `proof.go`

Handles Merkle receipt fetching and validation:

* `FetchProof()` → queries an account and retrieves its `Receipt`
* `VerifyProof()` → checks if the receipt's anchor matches the known BPT root and validates its path
* `ValidateAndCacheProof()` → full flow that verifies and stores valid proofs in cache

### 5. `cache.go`

Maintains in-memory caches for:

* Account proofs
* Heights
* Receipt objects

It uses `sync.RWMutex` for thread-safe access.

### 6. `errors.go`

Centralizes all error codes and types, such as:

* `ErrInvalidAccountURL`, `ErrProofValidation`, `ErrNetworkFailure`
* Also provides helper functions like `IsNotFound`, `IsValidationError`

### 7. `types.go`

Defines:

```go
type Transaction struct {
	TxID, Type, Status, Amount, From, To, Account string
	Timestamp, Height int64
	Data interface{}
}
```

Used across caching and transaction querying.

---

## Connecting to the Kermit Testnet

In `account_data_test.go`, all functional tests run against the **Kermit** testnet:

```go
const kermitAPI = "https://testnet.accumulatenetwork.io"
```

This connects the `LiteClient` to a real Accumulate node using the v2 API.

## Why Kermit?

* Kermit is a public testnet with live data and ACME token accounts for development.
* It allows end-to-end integration testing without the risk of using real tokens or modifying mainnet data.

---

## Test Coverage – `account_data_test.go`

This test file includes two main test functions:

### ✅ `TestAccountDataAPI_Features`

Covers:

1. `GetBalance` – queries balance for a known testnet account
2. `GetTransactions` – fetches last 10 transactions
3. `GetBalanceAndTransactions` – checks for consistency

Also verifies:

* Structure of the returned data
* Data integrity (e.g., matching `AccountUrl`, valid height)

### ❌ `TestAccountDataAPI_ErrorHandling`

Covers invalid account input:

* Uses a fake URL (`acc://invalid/account`)
* Ensures `GetBalance` and `GetTransactions` return errors
* Confirms robust error propagation

---

## How It Calls the Full Accumulate Client (Not Lite Client)

The `LiteClient` directly uses:

* `v2.New()` to instantiate a **full v2 API client**
* Then uses `RequestAPIv2()` to query endpoints like `"query"`, `"query-tx-history"`

This is different from the "lite client" mode, which would use on-chain proofs only. Here, the full node is queried for the data, then verified with the lite client logic.

---

## How It Ensures Correctness

The client follows a dual-phase validation:

1. **Proof Validation**

   * Uses `FetchBPTRootHash()` to get BPT root
   * Retrieves Merkle proof from `FetchProof()`
   * Verifies receipt using `VerifyProof()`
   * Caches if valid

2. **Data Integrity**

   * After proof validation, balance and transaction data is fetched
   * Structural checks in test cases confirm field validity
   * Caching ensures consistent access

## Conclusion

This Account API provides a robust, extensible foundation for interacting with token accounts on the Accumulate network. It bridges full node queries with lightweight validation and caching. With some modularization and v3 enhancements, it can evolve into a production-grade lite client solution.

---

Let me know if you want this turned into a PDF, integrated into docs, or split across multiple markdown files.
