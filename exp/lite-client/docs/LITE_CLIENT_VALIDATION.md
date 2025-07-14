# Lite Client Major Block Validation

This document provides a comprehensive overview of the major block validation process within the Accumulate lite client. The process is designed to cryptographically verify the authenticity and integrity of major blocks, allowing the lite client to trust the network's state without processing the entire blockchain.

## Validation Workflow

The validation process is orchestrated through a series of stages, each handled by specialized components. This ensures a clear separation of concerns and a robust, verifiable system.

### 1. Initiation and Data Extraction (`validate.go`)

The validation process begins with a call to `ValidateMajorBlock`. This function serves as the primary entry point and orchestrates the validation workflow.

-   **Input**: It takes a `MajorBlockRecord` (the block to be validated) and a `BlockValidator` instance.
-   **Anchor and Signature Extraction**: The function traverses the structure of the major block to find two critical pieces of data:
    1.  **Block Anchor**: It searches for a `BlockAnchor` message. This message contains the root hash of the block's transactions, which is the data that validators sign. The first anchor found is used for validation. If no anchor is present, the block is considered invalid.
    2.  **Signatures**: It collects all cryptographic signatures associated with the block. These signatures are typically found within `SignatureSetRecord` or attached to other messages.
2.  **Extract Anchor and Query Signatures (v3 API)**: For each major block retrieved, the client must find the `BlockAnchor` message within its transaction list. The hash of this anchor is then used to query the v3 API's `MessageHashSearchQuery` endpoint. This v3 query returns the full `MessageRecord`, which contains the `SignatureSet`—the complete set of validator signatures for that block.

3.  **Verify Signatures Against Authorities**: The retrieved signatures are then verified against the known set of authorities for that block height. The number of valid signatures must meet or exceed the consensus threshold (e.g., 2/3+).

This hybrid v2/v3 approach is the only viable path forward until the v3 API provides a direct way to query major blocks with their full signature sets included.

## II. Implementation Status & Test Results Analysis

The current implementation is incomplete and partially broken. The following analysis is based on the test run executed on `2025-07-14`.

### What Works

-   **Querying Major Blocks (v2 API)**: `TestQueryMajorBlocksV2_Kermit` **PASSED**. The client can successfully connect to a node and retrieve major block headers using the v2 API. This confirms the first step of our validation strategy is functional.
-   **Authority Providers**: The `StaticAuthorityProvider` and `GenesisAuthorityProvider` are implemented, allowing the validator to manage authority sets, which is a foundational requirement.

### What is Broken

-   **Querying Major Blocks (v3 API)**: `TestQueryMajorBlocksV3_Kermit` **FAILED**.
    -   **Reason**: `unexpected response type: expected RecordRange[*api.MajorBlockRecord], got *api.RecordRange[api.Record]`.
    -   **Impact**: The client cannot correctly parse the response from the v3 major block endpoint. This is a critical failure that blocks any validation path relying solely on the v3 API for block data.

-   **End-to-End Mock Validation**: `TestPhase2_EndToEndValidation` **FAILED**.
    -   **Reason**: `field Type: not equal: want ed25519, got unknown`.
    -   **Impact**: The test, which uses a fully mocked environment, fails because the signature type is not being correctly identified during validation. This points to a bug in how signatures are constructed in the mock or processed by the validator.

-   **Core Signature Validation**: `TestValidateMajorBlock_Success` **FAILED**.
    -   **Reason**: `not enough signatures: got 0, want 1`.
    -   **Impact**: This is the most critical failure. The validator is unable to find a single valid signature in a test case designed to be a success. This indicates a fundamental flaw in the `BlockValidator`'s logic for iterating signatures, verifying them against public keys, or matching them with the correct authorities.

-   **Test Setup for Unauthorized Signer**: `TestValidateMajorBlock_UnauthorizedSigner` **FAILED** (Panic).
    -   **Reason**: `panic: ed25519: bad seed length: 33`.
    -   **Impact**: The test itself is broken. It is attempting to generate an ed25519 key with an invalid seed, causing a panic. While this doesn't reflect a bug in the validation logic, it prevents us from testing a crucial security scenario.

## III. Next Steps

To complete Phase 2, the following issues must be addressed in order of priority:

1.  **Fix the Core Validation Logic**: The highest priority is to fix the `BlockValidator` so that `TestValidateMajorBlock_Success` passes. The logic must be corrected to properly iterate through the `SignatureSet`, verify each signature against the anchor hash, and confirm the signer is a valid authority.

2.  **Fix the End-to-End Mock Test**: Debug the `TestPhase2_EndToEndValidation` failure. Ensure that mock `ED25519Signature` objects are created with the correct type information and that the validator can correctly interpret it.

3.  **Fix the `TestValidateMajorBlock_UnauthorizedSigner` Test**: Correct the test setup by providing a valid seed to `ed25519.NewKeyFromSeed` to ensure the unauthorized signer scenario can be tested.

4.  **Address the v3 Block Query Failure**: While our primary path uses the v2 API, the failing `TestQueryMajorBlocksV3_Kermit` should be fixed to ensure future compatibility and provide an alternative data path. This involves correcting the client's response parsing logic.

5.  **Implement the Full Workflow**: Once the core validation logic is fixed and tested, the final step is to orchestrate the full hybrid workflow: fetch blocks via v2, extract anchors, fetch signatures via v3, and run the validator.
