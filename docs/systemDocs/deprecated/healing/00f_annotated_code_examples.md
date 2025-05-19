# Annotated Code Examples for Accumulate Healing

<!-- AI-METADATA
type: code_examples
version: 1.0
topic: healing_implementation
subtopics: ["on_demand_transaction_fetching", "error_handling", "transaction_discovery"]
related_code: ["internal/core/healing/synthetic.go", "internal/core/healing/anchors.go", "internal/core/healing/anchor_synth_report_test.go"]
critical_rules: ["never_fabricate_data", "follow_reference_implementation"]
tags: ["healing", "code_examples", "implementation", "ai_optimized"]
-->

## Overview

This document provides annotated code examples from the Accumulate healing implementation, focusing on critical patterns and algorithms. Each example includes detailed annotations to help AI systems understand the implementation details and best practices.

## On-Demand Transaction Fetching

The following example demonstrates the reference implementation for on-demand transaction fetching, which is the canonical pattern that must be followed exactly:

```go
// CRITICAL IMPLEMENTATION: This is the reference pattern for on-demand transaction fetching
// that must be followed exactly to maintain data integrity.
// Based on the implementation in anchor_synth_report_test.go
func fetchTransaction(ctx context.Context, client api.Querier, txid *url.TxID, args *HealSyntheticArgs) (*protocol.Transaction, error) {
    // PATTERN: First try to get the transaction from the known transactions map
    // This is an optimization to avoid network calls for already known transactions
    if txn, ok := args.TxnMap[txid.Hash()]; ok {
        slog.InfoContext(ctx, "Found transaction in cache", "txid", txid)
        return txn, nil
    }
    
    // PATTERN: If not in the map, try to fetch it from the network
    // Use the tryEach method to attempt multiple peers with increasing timeouts
    Q := api.Querier2{Querier: args.tryEach()}
    r, err := Q.QueryTransaction(ctx, txid, nil)
    
    // CRITICAL ERROR HANDLING: Intercept "key not found" errors
    // This is where on-demand fetching begins
    if errors.Is(err, errors.NotFound) {
        // Log the error for observability
        slog.InfoContext(ctx, "Transaction not found, attempting to fetch by sequence", "txid", txid)
        
        // PATTERN: Use sequence number for targeted fetching
        // This is more efficient than scanning all transactions
        srcUrl := protocol.PartitionUrl(args.Source)
        dstUrl := protocol.PartitionUrl(args.Destination)
        
        // Get the synthetic ledger to find the sequence
        synthLedger := getAccount[*protocol.SyntheticLedger](args, srcUrl.JoinPath(protocol.Synthetic))
        if synthLedger == nil {
            // CRITICAL: Do not fabricate data if we can't get the ledger
            return nil, errors.NotFound.WithFormat("synthetic ledger not found for %s", srcUrl)
        }
        
        // Find the sequence for the destination
        seq := synthLedger.Sequence(dstUrl)
        if seq == nil || seq.Count < args.Number {
            // CRITICAL: Do not fabricate data if sequence doesn't exist
            return nil, errors.NotFound.WithFormat("sequence %d not found for %s→%s", args.Number, args.Source, args.Destination)
        }
        
        // PATTERN: Fetch the transaction by sequence number
        r, err = Q.QueryMessageBySequence(ctx, srcUrl, dstUrl, args.Number, nil)
        if err != nil {
            // CRITICAL: Do not fabricate data if we can't get the transaction
            return nil, errors.Wrap(err, "failed to fetch transaction by sequence")
        }
        
        // Extract the transaction from the message
        msg, ok := r.Message.(messaging.MessageForTransaction)
        if !ok {
            // CRITICAL: Do not fabricate data if message is not a transaction
            return nil, errors.UnknownError.WithFormat("message is not a transaction: %T", r.Message)
        }
        
        txn := msg.GetTransaction()
        
        // PATTERN: Verify the transaction hash matches the expected hash
        if txid != nil && !bytes.Equal(txn.GetHash(), txid.Hash()) {
            // CRITICAL: Do not return incorrect data
            return nil, errors.UnknownError.WithFormat("fetched transaction hash does not match expected hash")
        }
        
        // PATTERN: Add the transaction to the map for future reference
        args.TxnMap[txn.GetHash()] = txn
        return txn, nil
    } else if err != nil {
        // Handle other errors with detailed logging
        slog.ErrorContext(ctx, "Failed to query transaction", "txid", txid, "error", err)
        return nil, errors.Wrap(err, "failed to query transaction")
    }
    
    // Extract the transaction from the successful query
    txn := r.Transaction
    
    // Add to cache for future reference
    args.TxnMap[txn.GetHash()] = txn
    return txn, nil
}
```

## Transaction Discovery Algorithm

The following example demonstrates the binary search algorithm used to efficiently discover missing transactions:

```go
// This function implements a binary search algorithm to efficiently find gaps in transaction sequences
func discoverMissingTransactions(ctx context.Context, client api.Client, args *HealSyntheticArgs) ([]*url.TxID, error) {
    // PATTERN: Start with the known range of sequence numbers
    start := args.StartSequence
    end := args.EndSequence
    
    // PATTERN: Binary search initialization
    missing := make([]*url.TxID, 0)
    
    // PATTERN: Binary search implementation
    // This is more efficient than linear scanning for large ranges
    for start <= end {
        mid := start + (end-start)/2
        
        // PATTERN: Check if the transaction at the middle sequence exists
        txn, err := client.QueryTransactionBySequence(ctx, &api.QueryTransactionBySequenceRequest{
            Sequence: mid,
        })
        
        if errors.Is(err, errors.NotFound) {
            // PATTERN: If transaction is missing, add it to the missing list
            missing = append(missing, &url.TxID{
                Hash: calculateHashForSequence(mid),
            })
            
            // PATTERN: Continue binary search in both directions
            // This handles cases where multiple transactions might be missing
            leftMissing, err := discoverMissingTransactions(ctx, client, &HealSyntheticArgs{
                StartSequence: start,
                EndSequence:   mid - 1,
            })
            if err != nil {
                return nil, errors.Wrap(err, "failed to discover missing transactions in left range")
            }
            missing = append(missing, leftMissing...)
            
            rightMissing, err := discoverMissingTransactions(ctx, client, &HealSyntheticArgs{
                StartSequence: mid + 1,
                EndSequence:   end,
            })
            if err != nil {
                return nil, errors.Wrap(err, "failed to discover missing transactions in right range")
            }
            missing = append(missing, rightMissing...)
            
            // Exit the loop after processing both sides
            break
        } else if err != nil {
            // Handle other errors
            return nil, errors.Wrap(err, "failed to query transaction by sequence")
        }
        
        // PATTERN: Transaction exists, update search range
        if txn.Sequence < args.TargetSequence {
            // Search right half
            start = mid + 1
        } else {
            // Search left half
            end = mid - 1
        }
    }
    
    return missing, nil
}
```

## Merkle Receipt Construction

The following example demonstrates how cryptographic receipts are built for transaction validation:

```go
// This function builds a Merkle receipt to cryptographically verify a transaction
func buildReceipt(ctx context.Context, client api.Client, txid *url.TxID) (*protocol.Receipt, error) {
    // PATTERN: Fetch the transaction's chain ID
    chainID, err := getChainID(ctx, client, txid)
    if err != nil {
        return nil, errors.Wrap(err, "failed to get chain ID")
    }
    
    // PATTERN: Fetch the Merkle state for the chain
    state, err := client.QueryChainState(ctx, &api.QueryChainStateRequest{
        ChainID: chainID,
    })
    if err != nil {
        return nil, errors.Wrap(err, "failed to query chain state")
    }
    
    // PATTERN: Build the Merkle path from the transaction to the chain root
    receipt, err := client.QueryReceipt(ctx, &api.QueryReceiptRequest{
        TxID:    txid.Hash(),
        ChainID: chainID,
    })
    if err != nil {
        return nil, errors.Wrap(err, "failed to query receipt")
    }
    
    // PATTERN: Verify the receipt is valid
    // This ensures cryptographic integrity
    err = receipt.Verify()
    if err != nil {
        return nil, errors.Wrap(err, "receipt verification failed")
    }
    
    return receipt, nil
}
```

## Signature Collection and Verification

The following example demonstrates how signatures are collected and verified for anchor healing:

```go
// This function collects and verifies signatures from validators for anchor healing
func collectSignatures(ctx context.Context, client api.Client, anchor *protocol.Anchor) ([]*protocol.Signature, error) {
    // PATTERN: Get the list of validators for the source partition
    validators, err := client.QueryValidators(ctx, &api.QueryValidatorsRequest{
        Partition: anchor.Source,
    })
    if err != nil {
        return nil, errors.Wrap(err, "failed to query validators")
    }
    
    // PATTERN: Initialize signature collection
    signatures := make([]*protocol.Signature, 0, len(validators.Validators))
    
    // PATTERN: Collect signatures from each validator
    for _, validator := range validators.Validators {
        // PATTERN: Query the signature from the validator
        sig, err := client.QuerySignature(ctx, &api.QuerySignatureRequest{
            Validator: validator.ID,
            AnchorID:  anchor.ID,
        })
        
        if err != nil {
            // Log the error but continue with other validators
            // This handles the case where some validators might be offline
            log.WithField("validator", validator.ID).WithError(err).Debug("Failed to get signature from validator")
            continue
        }
        
        // PATTERN: Verify the signature is valid for this anchor
        err = sig.Verify(anchor.ID)
        if err != nil {
            // Log the error but continue with other validators
            // This handles the case where a validator might provide an invalid signature
            log.WithField("validator", validator.ID).WithError(err).Debug("Invalid signature from validator")
            continue
        }
        
        // Add the valid signature to the collection
        signatures = append(signatures, sig)
    }
    
    // PATTERN: Ensure we have enough signatures for consensus
    // This is typically 2/3 of validators in Byzantine fault tolerance
    if len(signatures) < requiredSignatures(len(validators.Validators)) {
        return nil, errors.New("insufficient valid signatures collected")
    }
    
    return signatures, nil
}
```

## Error Handling Patterns

The following example demonstrates the structured error handling patterns used throughout the healing implementation:

```go
// This function demonstrates the error handling patterns used in healing
func processWithErrorHandling(ctx context.Context, client api.Client, txid *url.TxID) error {
    // PATTERN: Context-aware error handling
    // This ensures operations can be canceled or timed out
    select {
    case <-ctx.Done():
        return errors.Wrap(ctx.Err(), "context canceled or timed out")
    default:
        // Continue processing
    }
    
    // PATTERN: Structured error creation
    // This provides rich context for debugging
    if txid == nil {
        return errors.BadRequest.With("transaction ID cannot be nil")
    }
    
    // PATTERN: Error wrapping
    // This preserves the call stack while adding context
    txn, err := client.QueryTransaction(ctx, &api.QueryTransactionRequest{
        Txid: txid.Hash(),
    })
    if err != nil {
        return errors.Wrap(err, "failed to query transaction").With("txid", txid)
    }
    
    // PATTERN: Error type checking
    // This allows specific handling for different error types
    if err := validateTransaction(txn); err != nil {
        if errors.Is(err, errors.ValidationError) {
            // Handle validation errors specifically
            return errors.Wrap(err, "transaction validation failed")
        }
        // Handle other errors
        return errors.Wrap(err, "transaction processing failed")
    }
    
    // PATTERN: Retry logic with exponential backoff
    // This handles transient network issues
    err = retry.WithBackoff(func() error {
        return submitTransaction(ctx, client, txn)
    }, retry.Attempts(3), retry.Exponential(time.Second))
    if err != nil {
        return errors.Wrap(err, "failed to submit transaction after retries")
    }
    
    return nil
}
```

## Usage for AI Systems

AI systems can use these annotated code examples to:

1. **Understand implementation patterns** - Each example includes detailed annotations explaining the purpose and patterns
2. **Identify critical sections** - Critical implementation details are clearly marked
3. **Learn error handling strategies** - Error handling patterns are explicitly documented
4. **Recognize algorithm structures** - Key algorithms like binary search are explained

When generating or analyzing code related to Accumulate healing, AI systems should reference these examples to ensure adherence to the established patterns and critical rules.
