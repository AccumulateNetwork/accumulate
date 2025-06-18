# Accumulate Lite Client – `account_proof.go` Module Documentation

This document explains the design, purpose, implementation, and test coverage for the `account_proof.go` module. The file is central to Phase 1 of the Lite Client, enabling cryptographic validation of account existence in the Accumulate blockchain using Merkle proofs derived from the Binary Patricia Tree (BPT).

---

## Purpose

This module allows external clients—like light nodes, auditors, or mobile verifiers—to verify the inclusion of a blockchain account in a specific state without full node access. It accomplishes this by generating portable Merkle proofs that can be independently verified against a known root hash.

---

## Core Data Structure

### `AccountProof`

| Field        | Type       | Description                                |
| ------------ | ---------- | ------------------------------------------ |
| `AccountUrl` | `string`   | Human-readable Accumulate account URL      |
| `LeafHash`   | `[]byte`   | Hash representing the leaf node in the BPT |
| `Siblings`   | `[][]byte` | Ordered Merkle siblings from leaf to root  |
| `RootHash`   | `[]byte`   | BPT root hash that the proof should match  |

This structure encapsulates all the cryptographic data needed to verify account inclusion.

---

## Key Functions

### `CreateAccountProof(batch *database.Batch, accountUrl string)`

* Parses the account URL
* Builds the database key for the BPT
* Gets the Merkle receipt (inclusion path) using `batch.BPT().GetReceipt`
* Extracts:
    * `LeafHash` (start of the receipt)
    * `Siblings` (receipt entries)
    * `RootHash` (current BPT root)
* Returns an `AccountProof` struct

### `VerifyAccountProof(p *AccountProof)`

* Uses `LeafHash` and `Siblings` to recompute the Merkle root
* Returns whether the computed root matches `RootHash`

### `extractSiblingsFromReceipt(receipt *merkle.Receipt)`

* Converts `receipt.Entries` into a `[][]byte` slice of sibling hashes

---

## Design Decisions

| Design Element               | Rationale                                          | Alternatives                                   |
| ---------------------------- | -------------------------------------------------- | ---------------------------------------------- |
| Stateless `AccountProof`     | Enables cross-system and offline verification      | Could embed more metadata (e.g., block height) |
| Separate create/verify funcs | Clear API separation, aids testing and reusability | Could combine into a proof object with methods |
| Use of `record.NewKey`       | Guarantees canonical key computation per protocol  | Make derivation pluggable in the future        |
| Hash-only verification       | Keeps lite clients lean and fast                   | Future: direction-aware or compressed proofs   |
| Direct error messages        | Simplifies debugging and logging                   | Optional: custom error types or codes          |

---

## Test Coverage Summary

| Test Function                           | What it Validates                                     |
| --------------------------------------- | ----------------------------------------------------- |
| `TestParseAccountUrl_Valid`             | Valid Accumulate URLs parse correctly                 |
| `TestParseAccountUrl_Invalid`           | Invalid URL strings return appropriate errors         |
| `TestCreateAccountProof_ValidAccount`   | Proof generation works for one and two accounts       |
| `TestCreateAccountProof_MissingAccount` | Returns error when account is missing                 |
| `TestVerifyAccountProof_Correct`        | A valid proof verifies successfully                   |
| `TestVerifyAccountProof_Incorrect`      | A tampered proof fails verification                   |
| `TestExtractSiblingsFromReceipt`        | Siblings are extracted correctly from Merkle receipt  |
| `TestCreateAccountProof_RealisticData`  | Multiple realistic account types are handled properly |

---

## Sample Test Case: Multi-Account Proof

When testing with two accounts (e.g., `acc://alice`, `acc://bob`), the Merkle path includes one sibling. This confirms that branching behavior is correctly captured and included in the proof. It helps ensure that the module scales correctly as more accounts are added.

## Conclusion

The `account_proof.go` module fulfills the goals of Phase 1 of the Lite Client by:

* Providing cryptographic inclusion guarantees
* Enabling stateless and offline verification
* Maintaining a clear separation between proof creation and validation
* Covering all edge cases and expected failures through unit tests

It is modular, minimal, and ready for integration into more advanced phases like snapshotting, range proofs, and state synchronization.
