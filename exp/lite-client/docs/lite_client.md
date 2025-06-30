# Lite Client for Accumulate

## Overview

This document outlines the design and implementation of a lite client for the Accumulate blockchain. The lite client is designed to provide a lightweight method to validate the integrity and authenticity of the Accumulate blockchain without requiring a full node.

## Purpose

The lite client will:

1. Create cryptographic proofs based on the BPT root hash for a set of account states.
2. Validate signatures from the genesis block to the current major block
3. Validate the signatures for the minor blocks to the present block
4. Create the cryptographic receipt to the root hash that covers the BPT root hash.
5. Collect the hashes and transactions for the set of accounts

### Understanding the Dynamic Proof Process

A blockchain is dynamic. It continues to produce minor blocks at a rapid pace, which means the state of the BPT is constantly changing. This creates a challenge for proof creation:

1. **Time-Sensitive Account Proofs**: We need to create proofs of account states against a BPT root hash quickly because the BPT will be modified frequently, causing all account state proofs to change.

2. **Block Chain Structure**: The BPT root hash is included as a component in every minor block. This allows us to create a proof chain from minor blocks back to genesis.

3. **Proof Flexibility**: The BPT root hash can be proven against any state hash for any minor block created afterward. This means the cryptographic proof of the BPT we collected earlier can be completed against any following minor block hash within a partition.

4. **Cross-Partition Anchoring**: The partition hash is anchored in the Directory Node. A proof to a minor block on the Directory Node completes the entire proof of a partition.

5. **Complete Proof Path**: At this point, we have a proof of an account state all the way to a minor block in the Directory Node.

6. **Continuous Updates**: As the blockchain progresses, we create more recent proofs to more recent Directory minor blocks. While the account proofs are never completely a proof of the absolute current state, we keep the latency between proved states and the current state very low.

## Implementation Plan

### Phase 1: Account State Proof Creation
*Aligns with Purpose Step 1: Create cryptographic proofs based on the BPT root hash for a set of account states.*

- Create cryptographic proofs for account states:
  1. Identify the set of accounts to be validated
  2. For each account, generate a cryptographic proof path in the BPT
  3. Ensure each proof connects the account state to the BPT root hash
  4. Verify these proofs are cryptographically sound and verifiable

### Phase 2: Major Block Signature Validation
*Aligns with Purpose Step 2: Validate signatures from the genesis block to the current major block.*

- Implement signature validation from genesis to the latest major block:
  1. Retrieve the genesis block and its authority
  2. For each major block, verify its signature against the authority of its time
  3. Track authority changes and update the verification chain accordingly
- Implement major block validation:
  1. Verify each major block's index is sequential
  2. Validate that each block's timestamp aligns with the major block schedule
  3. Verify anchoring data for each major block

### Phase 3: Minor Block Signature Validation
*Aligns with Purpose Step 3: Validate the signatures for the minor blocks to the present block.*

- Implement minor block validation from the latest major block to the present:
  1. Verify the chain of minor blocks following the last major block
  2. Validate signatures and state transitions for each minor block
  3. Ensure proper linking between minor blocks and their parent major block

### Phase 4: Root Hash Receipt Creation
*Aligns with Purpose Step 4: Create the cryptographic receipt to the root hash that covers the BPT root hash.*

- Create cryptographic receipts for the root hash:
  1. Generate receipts that connect the BPT root hash to the block chain
  2. Verify the receipt path from the BPT root to the block headers
  3. Validate the integrity of the receipt chain
  4. Ensure the receipt provides a complete validation path

### Phase 5: Account Transaction Collection
*Aligns with Purpose Step 5: Collect the hashes and transactions for the set of accounts.*

- Collect and validate transaction data:
  1. Gather all transactions for the specified accounts
  2. Verify transaction inclusion in their respective blocks
  3. Validate transaction hashes against the block data
- Develop a comprehensive validation report summarizing all verification steps

## Technical Details

### Major Block Structure and Validation

Major blocks in Accumulate represent significant checkpoints in the blockchain's history and are constructed as follows:

#### Scheduling and Triggering
- Major blocks are scheduled according to a cron expression defined in `NetworkGlobals.MajorBlockSchedule`
- The Directory Network (DN) is responsible for initiating major blocks via the `shouldOpenMajorBlock()` function
- The scheduling logic checks if the current block time exceeds the next scheduled major block time

#### Construction Process
- When it's time for a major block, the DN sets `block.State.MajorBlock` with a new index and timestamp
- The DN sends a `MakeMajorBlock` message to all Block Validator Networks (BVNs)
- BVNs process this message in `msg_make_major_block.go` and update their state accordingly
- The major block index is incremented from the previous major block index

#### Block Validation and Authority
- Each major block is signed by the authority of its time using the `signTransaction()` function
- Authority changes are recorded in the blockchain and must be tracked by the lite client
- The lite client must verify that each signature was created by the correct authority for that time period

#### Root Hash Validation
- Root hashes are stored in the Binary Patricia Tree (BPT)
- The previous state hash is captured at the beginning of each block using `block.Batch.GetBptRootHash()`
- Root hash receipts are generated using the `getRootReceiptForBlock()` function
- These receipts create a verifiable chain from one block to another

#### Anchoring Process
- Major blocks are anchored using the `recordMajorBlock()` function
- This creates index entries that link the major block to the main chain
- The anchoring process ensures the integrity of the blockchain by creating a verifiable chain of signatures
- The `recordMajorBlock()` function updates the major index chains with entries containing:
  - Source: The height of the main chain
  - RootIndexIndex: The index of the root anchor index chain entry
  - BlockIndex: The major block index
  - BlockTime: The timestamp of the major block

#### Major Block Validation Process

The lite client will validate major blocks from genesis to the present using the following step-by-step process:

1. **Retrieve Genesis Block Information**
   - API: `client.QueryMajorBlocks(ctx, query)` where query contains:
     - URL: `acc://bvn0.acme` (specific partition URL)
     - Count: 1
     - Start: 0
   - Obtain the genesis block's index, timestamp, and minor blocks
   - Important: Use a specific partition URL (like bvn0.acme) rather than the Directory Network URL

2. **Retrieve Major Block Chain**
   - Use the same API call with pagination to retrieve multiple blocks
   - API: `client.QueryMajorBlocks(ctx, query)` where query contains:
     - URL: `acc://bvn0.acme` (specific partition URL)
     - Count: 10 (or desired number of blocks per page)
     - Start: lastProcessedIndex
   - Process blocks in batches to build the complete chain

3. **For Each Major Block:**
   - Extract data from the response using JSON marshaling/unmarshaling
   - Important fields include:
     - majorBlockIndex: The block's index
     - majorBlockTime: The block's timestamp
     - minorBlocks: Array of minor blocks contained in the major block

4. **Validate Block Signatures**
   - For each signature on the major block:
     - Verify the signature against the known authority public key
     - Ensure the signature covers the block's root hash
     - API: `protocol.VerifySignature(signature, rootHash, publicKey)`

5. **Validate Block Sequence**
   - Verify each major block's index is sequential
   - Verify timestamps follow the major block schedule
   - API: `client.QueryNetworkGlobals()` to retrieve the block schedule

6. **Track Authority Changes**
   - When an authority change is detected in a major block:
     - API: `client.QueryTransaction(authorityChangeTransaction)`
     - Verify the authority change transaction is properly signed by the previous authority
     - Update the tracked authority set for validating subsequent blocks

7. **Validate Root Hash Chain**
   - For each major block:
     - API: `client.QueryReceipt(majorBlockRootHash)`
     - Verify the receipt connects the current block to the previous block
     - Validate the merkle path in the receipt

8. **Generate Validation Report**
   - Create a summary of the validation results
   - Include any detected anomalies or verification failures

### Lite Client Implementation

The lite client is a standalone application that uses cryptographic proofs to validate a small set of accounts without needing to validate the entire Accumulate blockchain. Key aspects include:

- **Selective Validation**: The lite client only validates the specific data paths needed for the accounts of interest, rather than processing the entire blockchain

- **Proof-Based Verification**: Uses cryptographic proofs (merkle receipts) to verify account states against validated major block root hashes

- **Minimal Resource Requirements**: Designed to run on consumer hardware with minimal storage and processing requirements

- **No Data Fabrication**: CRITICAL RULE - Data retrieved from the Protocol CANNOT be faked. Doing so masks errors and leads to huge wastes of time for those monitoring the Network. All data must be obtained directly from the network without modification.

- **API Utilization**: Leverages the following Accumulate v2 APIs:
  - `client.QueryMajorBlocks()`: Retrieve major block information using specific partition URLs (e.g., `acc://bvn0.acme`)
  - `client.Version()`: Check connectivity to network endpoints
  - `client.QueryTransaction()`: Get transaction details when needed
  - `client.QueryReceipt()`: Get merkle receipts for validation

- **Proper Fallback Mechanisms**: Implement fallback mechanisms as defined in reference test code without fabricating data

- **Detailed Logging**: Add comprehensive logging to track API calls, responses, and any errors

- **Offline Verification**: Once proofs are obtained, verification can be performed offline without continuous network connectivity

- **Trust Minimization**: Does not require trusting any single node; verifies all data cryptographically

## Usage Scenarios

### Blockchain Auditing
The lite client can be used to audit the entire blockchain, verifying that all authority changes and signatures are valid from genesis to the present.

### Account Verification
Users can verify the state of their accounts without trusting a full node, by validating the account state against the verified BPT.

### Network Health Monitoring
The lite client can be used to monitor the health of the network by regularly validating the latest blocks and state.

## Critical Development Rules

1. **DO NOT SKIP TESTS TO FIX THEM**
   - Tests must be fixed properly rather than being skipped
   - Skipping tests masks underlying issues and leads to undetected problems
   - Always solve the root cause of test failures

2. **DO NOT FAKE DATA**
   - Data retrieved from the Protocol CANNOT be faked
   - Faking data masks errors and leads to huge wastes of time for those monitoring the Network
   - The only acceptable implementation is to follow exactly what's in the reference test code
   - Any data collection must match the test implementation precisely, including fallback mechanisms

3. **FOLLOW REFERENCE IMPLEMENTATIONS**
   - When implementing features like on-demand transaction fetching, follow the reference test code exactly
   - Intercept specific errors (like "key not found") and handle them as specified in the reference code
   - Add detailed logging to track when transactions are fetched on-demand

4. **USE SPECIFIC PARTITION URLS**
   - When querying major blocks, use specific partition URLs (e.g., `acc://bvn0.acme`) rather than the Directory Network
   - The Directory Network URL often results in timeouts that are misleading
   - Always test connectivity to endpoints before making API calls

## Future Enhancements

- Integration with the Accumulate CLI for easy access
- Web interface for non-technical users
- Automated monitoring and alerting system
- Performance optimizations for large-scale validation
- Enhanced testing framework with comprehensive test coverage
