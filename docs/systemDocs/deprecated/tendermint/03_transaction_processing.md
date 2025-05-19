---
title: Transaction Processing with Tendermint in Accumulate
description: Detailed explanation of how transactions flow through Tendermint and Accumulate
tags: [tendermint, transactions, processing, consensus, execution]
created: 2025-05-16
version: 1.0
---

# Transaction Processing with Tendermint in Accumulate

## Introduction

Transaction processing in Accumulate involves a sophisticated interplay between Tendermint's consensus engine and Accumulate's application logic. This document provides a detailed explanation of how transactions flow through the system, from submission to execution and finalization.

## Transaction Lifecycle Overview

The lifecycle of a transaction in Accumulate follows these key stages:

1. **Submission**: Transaction is submitted to the network
2. **Routing**: Transaction is routed to the appropriate partition
3. **Validation**: Transaction is validated before entering the mempool
4. **Consensus**: Tendermint orders transactions and reaches consensus
5. **Execution**: Accumulate executes the transaction and updates state
6. **Finalization**: State changes are committed and results returned
7. **Anchoring**: Transaction results are secured through cross-chain anchoring

## Transaction Submission

### Client Submission

Transactions begin their journey when submitted by a client:

1. Client constructs a transaction with appropriate signatures
2. Transaction is wrapped in an envelope with metadata
3. Client submits the transaction to an Accumulate node via API
4. Node receives the transaction and begins processing

### API Handling

The API layer processes incoming transactions:

```go
func (s *Service) Submit(ctx context.Context, envelope *messaging.Envelope) ([]*api.SubmissionResponse, error) {
    // Normalize and validate the envelope
    // Route the transaction to the appropriate partition
    // Submit to Tendermint via RPC
    // Return submission response
}
```

## Transaction Routing

### Dispatcher Implementation

The Tendermint dispatcher routes transactions to the appropriate partition:

```go
func (d *dispatcher) Submit(ctx context.Context, u *url.URL, env *messaging.Envelope) error {
    // Normalize the envelope
    // Route the account to determine partition
    partition, err := d.router.RouteAccount(u)
    // Queue the envelope for the determined partition
    d.queue[partition] = append(d.queue[partition], env.Copy())
    return nil
}
```

### Partition Determination

Routing is based on the account URL:

1. Directory Network handles global routing and ADI registry
2. Block Validator Networks handle specific account types
3. Special handling for cross-partition transactions

## Transaction Validation (CheckTx)

Before entering the mempool, transactions undergo validation:

### Signature Verification

```go
func verifySignatures(tx *protocol.Transaction) error {
    // Extract signatures from transaction
    // Retrieve public keys from account
    // Verify each signature against transaction hash
    // Check if signature threshold is met
    // Return error if verification fails
}
```

### State Validation

```go
func validateTransaction(tx *protocol.Transaction, state State) error {
    // Check if transaction type is valid
    // Verify account exists and is of correct type
    // Check if account has sufficient balance (for token transfers)
    // Verify transaction complies with account rules
    // Return error if validation fails
}
```

### Mempool Inclusion

If validation succeeds, Tendermint adds the transaction to its mempool:

1. Transaction is stored in the mempool
2. Transaction is gossiped to peers
3. Transaction awaits inclusion in a block

## Consensus Processing

Tendermint handles transaction ordering and consensus:

### Block Proposal

1. Validator node becomes proposer based on round-robin selection
2. Proposer selects transactions from mempool
3. Proposer creates a block proposal
4. Proposal is broadcast to other validators

### Consensus Rounds

1. **Propose**: Proposer suggests a block
2. **Prevote**: Validators vote on the proposal
3. **Precommit**: Validators commit to the block
4. **Commit**: Block is finalized when 2/3+ voting power commits

### Block Finalization

Once consensus is reached:
1. Block is added to the blockchain
2. Tendermint delivers the block to the ABCI application
3. Accumulate processes the transactions in the block

## Transaction Execution

### BeginBlock Processing

At the start of each block:

```go
func (app *AccumulateApplication) BeginBlock(req abci.RequestBeginBlock) abci.ResponseBeginBlock {
    // Record block header information
    // Initialize state batch for the block
    // Process validator evidence (if any)
    // Prepare for transaction execution
}
```

### Transaction Execution (DeliverTx)

Each transaction is executed sequentially:

```go
func (app *AccumulateApplication) DeliverTx(req abci.RequestDeliverTx) abci.ResponseDeliverTx {
    // Decode transaction from bytes
    tx, err := protocol.UnmarshalTransaction(req.Tx)
    
    // Execute transaction based on type
    switch tx.Body.Type() {
    case protocol.TransactionTypeTokenTransfer:
        return executeTokenTransfer(tx, app.state)
    case protocol.TransactionTypeCreateAccount:
        return executeCreateAccount(tx, app.state)
    // Handle other transaction types
    }
}
```

### State Updates

During execution, the state is updated:

1. Account balances are modified for token transfers
2. New accounts are created or existing ones updated
3. Data is written to data accounts
4. Events are generated for indexing and client notifications

### EndBlock Processing

At the end of each block:

```go
func (app *AccumulateApplication) EndBlock(req abci.RequestEndBlock) abci.ResponseEndBlock {
    // Process any pending operations
    // Calculate validator updates (if needed)
    // Generate synthetic transactions for cross-chain operations
    // Return events and validator updates
}
```

## Transaction Finalization

### State Commitment

After all transactions are processed:

```go
func (app *AccumulateApplication) Commit() abci.ResponseCommit {
    // Finalize all state changes
    // Compute new state root hash
    // Commit changes to persistent storage
    // Return new state hash to Tendermint
}
```

### Result Handling

Transaction results are made available:

1. Success or failure status is recorded
2. Events are indexed for querying
3. Receipts are generated for clients
4. State changes become queryable

## Cross-Partition Transactions

Transactions that affect multiple partitions require special handling:

### Initial Processing

1. Transaction is processed in its primary partition
2. System generates synthetic follow-up transactions for other partitions

### Synthetic Transactions

```go
func generateSyntheticTransactions(tx *protocol.Transaction, result *protocol.TransactionResult) []*protocol.Transaction {
    // Determine if cross-partition effects are needed
    // Create synthetic transactions for other partitions
    // Include references to the original transaction
    // Return synthetic transactions for processing
}
```

### Transaction Anchoring

Transactions are secured through anchoring:

1. Each partition creates a Merkle root of its transactions
2. Root is anchored to the Directory Network
3. Directory Network anchors back to all partitions
4. Creates a cryptographic chain of trust

## Error Handling

Robust error handling is essential for transaction processing:

### Transaction Failures

When a transaction fails during execution:

1. Failure is recorded in the transaction result
2. State changes are reverted
3. Failure reason is logged
4. Client is notified of the failure

### System Recovery

For system-level failures:

1. Tendermint's consensus ensures agreement on transaction outcomes
2. Failed nodes can recover by syncing from peers
3. State machine ensures deterministic execution

## Transaction Querying

After processing, transactions can be queried:

### By Transaction ID

```go
func queryTransaction(txid []byte, state State) (*protocol.Transaction, *protocol.TransactionResult) {
    // Look up transaction by ID
    // Retrieve transaction data and result
    // Return to client
}
```

### By Account

```go
func queryAccountTransactions(accountUrl string, state State) []*protocol.Transaction {
    // Look up account's transaction history
    // Filter and sort transactions
    // Return transaction list
}
```

## Performance Considerations

Transaction processing is optimized for performance:

### Batching

- Transactions are processed in batches (blocks)
- State updates are batched for efficiency
- Database operations are optimized

### Parallel Processing

- Signature verification can be parallelized
- Different partitions process transactions in parallel
- Query processing is separate from consensus

### Caching

- Frequently accessed state is cached
- Validated transactions are cached in the mempool
- Account data is cached for quick access

## References

- [Accumulate Overview](../01_accumulate_overview.md)
- [Tendermint Integration Overview](../05_tendermint_integration.md)
- [Multi-Chain Structure](01_multi_chain_structure.md)
- [ABCI Application](02_abci_application.md)
- [Network Communication](04_network_communication.md)
