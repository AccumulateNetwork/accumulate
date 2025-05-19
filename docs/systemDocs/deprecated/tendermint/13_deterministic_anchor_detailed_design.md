---
title: Deterministic Anchor Transmission - Detailed Design and Migration Plan
description: Comprehensive technical design and migration plan for implementing deterministic anchor transmission in Accumulate
tags: [accumulate, anchoring, cross-network, deterministic, migration, implementation]
created: 2025-05-16
version: 1.0
---

# Deterministic Anchor Transmission - Detailed Design and Migration Plan

## Detailed Technical Design

### 1. Core Components

The deterministic anchor transmission system consists of the following core components:

#### 1.1 Deterministic Anchor Generator

This component is responsible for generating identical anchors across all validators.

**File:** `internal/core/crosschain/deterministic.go`

```go
// DeterministicAnchorGenerator is responsible for creating anchors that are
// identical across all validators given the same block state
type DeterministicAnchorGenerator struct {
    database    *database.Batch
    networkID   string
    partitionID string
    logger      logging.Logger
}

// NewDeterministicAnchorGenerator creates a new generator
func NewDeterministicAnchorGenerator(db *database.Batch, networkID, partitionID string, logger logging.Logger) *DeterministicAnchorGenerator {
    return &DeterministicAnchorGenerator{
        database:    db,
        networkID:   networkID,
        partitionID: partitionID,
        logger:      logger,
    }
}

// GenerateAnchor creates an anchor deterministically from the block state
func (g *DeterministicAnchorGenerator) GenerateAnchor(ctx context.Context, blockIndex uint64) (*protocol.PartitionAnchor, error) {
    g.logger.Debug("Generating deterministic anchor", "block", blockIndex, "partition", g.partitionID)
    
    // Load system ledger state at the specified block
    var systemLedger *protocol.SystemLedger
    err := g.database.Account(protocol.PartitionUrl(g.partitionID).JoinPath(protocol.Ledger)).
        MainChain().
        Entry(int64(blockIndex)).
        GetAs(&systemLedger)
    if err != nil {
        return nil, fmt.Errorf("failed to get system ledger: %w", err)
    }
    
    // Get the root chain state
    rootChain, err := g.database.Account(protocol.PartitionUrl(g.partitionID).JoinPath(protocol.Ledger)).
        RootChain().
        Get()
    if err != nil {
        return nil, fmt.Errorf("failed to get root chain: %w", err)
    }
    
    // Get the state root at this block
    stateRoot, err := g.database.GetBptRootHash(blockIndex)
    if err != nil {
        return nil, fmt.Errorf("failed to get state root: %w", err)
    }
    
    // Construct the anchor deterministically
    anchor := &protocol.PartitionAnchor{
        Source:          protocol.PartitionUrl(g.partitionID),
        RootChainIndex:  rootChain.Height(),
        RootChainAnchor: *(*[32]byte)(rootChain.Anchor()),
        StateTreeAnchor: stateRoot,
        MinorBlockIndex: systemLedger.Index,
        MajorBlockIndex: systemLedger.MajorBlockIndex,
        Timestamp:       systemLedger.Timestamp,
    }
    
    g.logger.Debug("Generated deterministic anchor", 
        "block", blockIndex, 
        "partition", g.partitionID,
        "rootChainIndex", anchor.RootChainIndex,
        "stateRoot", hex.EncodeToString(anchor.StateTreeAnchor[:8])+"...")
    
    return anchor, nil
}
```

#### 1.2 Deterministic Transaction Builder

This component creates identical transactions from anchors across all validators.

**File:** `internal/core/crosschain/deterministic.go`

```go
// DeterministicTxBuilder builds deterministic transactions from anchors
type DeterministicTxBuilder struct {
    logger logging.Logger
}

// NewDeterministicTxBuilder creates a new transaction builder
func NewDeterministicTxBuilder(logger logging.Logger) *DeterministicTxBuilder {
    return &DeterministicTxBuilder{
        logger: logger,
    }
}

// BuildAnchorTransaction builds a deterministic transaction from an anchor
func (b *DeterministicTxBuilder) BuildAnchorTransaction(anchor *protocol.PartitionAnchor, destination *url.URL) (*protocol.Transaction, error) {
    b.logger.Debug("Building deterministic transaction", 
        "source", anchor.Source, 
        "destination", destination)
    
    // Create transaction header
    txn := new(protocol.Transaction)
    txn.Header.Principal = destination.JoinPath(protocol.AnchorPool)
    
    // Set the body to the anchor
    txn.Body = anchor
    
    // Generate a deterministic transaction ID based on anchor content
    // This is critical - all validators must generate the same ID
    txHash := sha256.Sum256(append(anchor.StateTreeAnchor[:], []byte(fmt.Sprintf("%d", anchor.RootChainIndex))...))
    
    // Create a deterministic initiator URL
    initiator := url.URL{
        Authority: protocol.TxSyntheticId,
        Path:      fmt.Sprintf("/%x", txHash[:8]),
    }
    txn.Header.Initiator = &initiator
    
    // Set metadata
    txn.Header.Metadata = &protocol.TransactionMetadata{
        Priority: protocol.PriorityHigh, // Anchors are high priority
    }
    
    b.logger.Debug("Built deterministic transaction", 
        "txid", txn.ID().String(),
        "source", anchor.Source, 
        "destination", destination)
    
    return txn, nil
}
```

#### 1.3 Signature Manager

This component handles the signing and submission of signatures for anchor transactions.

**File:** `internal/core/crosschain/signatures.go` (new file to create)

```go
// SignatureManager handles signing and submitting signatures
type SignatureManager struct {
    validatorKey ed25519.PrivateKey
    validatorUrl *url.URL
    client       *api.Client
    logger       logging.Logger
}

// NewSignatureManager creates a new signature manager
func NewSignatureManager(validatorKey ed25519.PrivateKey, validatorUrl *url.URL, rpcEndpoint string, logger logging.Logger) *SignatureManager {
    client := api.NewClient(api.ClientOptions{
        URL: rpcEndpoint,
    })
    
    return &SignatureManager{
        validatorKey: validatorKey,
        validatorUrl: validatorUrl,
        client:       client,
        logger:       logger,
    }
}

// SignAndSubmit signs a transaction and submits the signature
func (m *SignatureManager) SignAndSubmit(ctx context.Context, txn *protocol.Transaction) error {
    txHash := txn.GetHash()
    m.logger.Debug("Signing transaction", "txid", txn.ID().String(), "hash", hex.EncodeToString(txHash[:8])+"...")
    
    // Create a signature builder
    builder := new(signing.Builder).
        SetType(protocol.SignatureTypeED25519).
        SetPrivateKey(m.validatorKey).
        SetUrl(m.validatorUrl).
        SetTimestampToNow()
    
    // Sign the transaction hash
    sig, err := builder.Sign(txHash[:])
    if err != nil {
        return fmt.Errorf("failed to sign transaction: %w", err)
    }
    
    // Create a signature message
    sigMsg := &messaging.SignatureMessage{
        TransactionHash: txHash,
        Signature:       sig,
    }
    
    // Create an envelope with just the signature
    env := &messaging.Envelope{
        Messages: []messaging.Message{sigMsg},
    }
    
    // Submit the signature via RPC
    m.logger.Debug("Submitting signature", "txid", txn.ID().String())
    _, err = m.client.Submit(ctx, env, api.SubmitOptions{
        Wait: api.BoolPtr(true), // Wait for confirmation
    })
    
    if err != nil {
        return fmt.Errorf("failed to submit signature: %w", err)
    }
    
    m.logger.Debug("Signature submitted successfully", "txid", txn.ID().String())
    return nil
}
```

#### 1.4 Transaction Monitor

This component monitors the status of submitted anchor transactions.

**File:** `internal/core/crosschain/monitor.go` (new file to create)

```go
// TransactionMonitor monitors the status of anchor transactions
type TransactionMonitor struct {
    client *api.Client
    logger logging.Logger
}

// NewTransactionMonitor creates a new transaction monitor
func NewTransactionMonitor(rpcEndpoint string, logger logging.Logger) *TransactionMonitor {
    client := api.NewClient(api.ClientOptions{
        URL: rpcEndpoint,
    })
    
    return &TransactionMonitor{
        client: client,
        logger: logger,
    }
}

// MonitorTransaction monitors a transaction until it's delivered or fails
func (m *TransactionMonitor) MonitorTransaction(ctx context.Context, txid *url.TxID) error {
    m.logger.Debug("Starting transaction monitoring", "txid", txid.String())
    
    // Set up polling with exponential backoff
    backoff := backoff.NewExponentialBackOff()
    backoff.InitialInterval = 500 * time.Millisecond
    backoff.MaxInterval = 10 * time.Second
    backoff.MaxElapsedTime = 5 * time.Minute
    
    // Poll for transaction status with backoff
    err := backoff.Retry(func() error {
        // Check if context is cancelled
        if ctx.Err() != nil {
            return backoff.Permanent(ctx.Err())
        }
        
        // Query transaction status
        status, err := m.client.QueryTransaction(ctx, txid, nil)
        if err != nil {
            if errors.Is(err, errors.NotFound) {
                // Transaction not found yet, continue polling
                m.logger.Debug("Transaction not found yet", "txid", txid.String())
                return errors.New("transaction not found")
            }
            return backoff.Permanent(fmt.Errorf("failed to query transaction: %w", err))
        }
        
        // Check if transaction is delivered
        if status.Status.Delivered() {
            m.logger.Debug("Transaction delivered successfully", "txid", txid.String())
            return nil
        }
        
        // Check for errors
        if status.Status.Error != nil {
            return backoff.Permanent(fmt.Errorf("transaction failed: %w", status.Status.Error))
        }
        
        // Continue polling
        m.logger.Debug("Transaction pending", "txid", txid.String(), "status", status.Status.Code)
        return errors.New("transaction pending")
    }, backoff)
    
    return err
}
```

#### 1.5 Anchor Orchestrator

This component orchestrates the entire anchor transmission process.

**File:** `internal/core/crosschain/orchestrator.go` (new file to create)

```go
// AnchorOrchestrator orchestrates the anchor transmission process
type AnchorOrchestrator struct {
    generator     *DeterministicAnchorGenerator
    txBuilder     *DeterministicTxBuilder
    sigManager    *SignatureManager
    txMonitor     *TransactionMonitor
    networkInfo   *protocol.NetworkDefinition
    partitionID   string
    logger        logging.Logger
}

// NewAnchorOrchestrator creates a new anchor orchestrator
func NewAnchorOrchestrator(
    generator *DeterministicAnchorGenerator,
    txBuilder *DeterministicTxBuilder,
    sigManager *SignatureManager,
    txMonitor *TransactionMonitor,
    networkInfo *protocol.NetworkDefinition,
    partitionID string,
    logger logging.Logger,
) *AnchorOrchestrator {
    return &AnchorOrchestrator{
        generator:   generator,
        txBuilder:   txBuilder,
        sigManager:  sigManager,
        txMonitor:   txMonitor,
        networkInfo: networkInfo,
        partitionID: partitionID,
        logger:      logger,
    }
}

// ProcessBlock processes a block for anchor transmission
func (o *AnchorOrchestrator) ProcessBlock(ctx context.Context, blockIndex uint64) error {
    o.logger.Info("Processing block for anchor transmission", "block", blockIndex, "partition", o.partitionID)
    
    // Generate the anchor deterministically
    anchor, err := o.generator.GenerateAnchor(ctx, blockIndex)
    if err != nil {
        return fmt.Errorf("failed to generate anchor: %w", err)
    }
    
    // Determine destinations based on partition type
    var destinations []*url.URL
    
    if o.partitionID == protocol.Directory {
        // DN -> BVNs
        for _, partition := range o.networkInfo.Partitions {
            if partition.Type == protocol.PartitionTypeBlockValidator {
                destinations = append(destinations, protocol.PartitionUrl(partition.ID))
            }
        }
    } else {
        // BVN -> DN
        destinations = append(destinations, protocol.DnUrl())
    }
    
    // Process each destination
    for _, destination := range destinations {
        // Add a small random delay to avoid network congestion
        // This helps prevent all validators from submitting at exactly the same time
        delay := time.Duration(rand.Intn(1000)) * time.Millisecond
        time.Sleep(delay)
        
        // Build the transaction
        txn, err := o.txBuilder.BuildAnchorTransaction(anchor, destination)
        if err != nil {
            o.logger.Error("Failed to build transaction", "error", err, "destination", destination)
            continue
        }
        
        // Sign and submit
        err = o.sigManager.SignAndSubmit(ctx, txn)
        if err != nil {
            o.logger.Error("Failed to sign and submit", "error", err, "txid", txn.ID())
            continue
        }
        
        // Monitor transaction asynchronously
        go func(txid *url.TxID) {
            monitorCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
            defer cancel()
            
            err := o.txMonitor.MonitorTransaction(monitorCtx, txid)
            if err != nil {
                o.logger.Error("Transaction monitoring failed", "error", err, "txid", txid)
            }
        }(txn.ID())
    }
    
    return nil
}
```

### 2. Integration with Accumulate Core

### 2.1 ABCI Application Integration

**File to modify:** [internal/node/abci/app.go](internal/node/abci/app.go)

The deterministic anchor transmission system needs to be integrated with Accumulate's ABCI application:

```go
// Add to internal/node/abci/end_block.go

// EndBlock is called at the end of processing a block
func (app *AccumulateApplication) EndBlock(ctx context.Context, req abcitypes.RequestEndBlock) abcitypes.ResponseEndBlock {
    // Existing end block logic
    response := app.BaseApplication.EndBlock(ctx, req)
    
    // Only process on validator nodes
    if app.Config.Validator.IsValidator {
        // Process the block for anchor transmission
        // This is done asynchronously to avoid blocking consensus
        go func(height int64) {
            // Create a new context with timeout
            ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
            defer cancel()
            
            // Get the orchestrator from the app
            orchestrator := app.AnchorOrchestrator
            
            // Process the block
            err := orchestrator.ProcessBlock(ctx, uint64(height))
            if err != nil {
                app.Logger.Error("Failed to process block for anchor transmission", 
                    "height", height, 
                    "error", err)
            }
        }(req.Height)
    }
    
    return response
}
```

#### 2.2 Node Configuration

**File to modify:** [internal/node/config/config.go](internal/node/config/config.go)

Update the node configuration to support deterministic anchor transmission:

```go
// Add to accumulate/internal/node/config.go

type Config struct {
    // Existing fields
    
    // Anchor transmission configuration
    AnchorTransmission struct {
        // Enable deterministic anchor transmission
        Enabled bool `json:"enabled" yaml:"enabled"`
        
        // RPC endpoint for submitting signatures
        RpcEndpoint string `json:"rpcEndpoint" yaml:"rpcEndpoint"`
        
        // Maximum number of blocks to process in parallel
        MaxParallelBlocks int `json:"maxParallelBlocks" yaml:"maxParallelBlocks"`
        
        // Maximum time to wait for transaction confirmation
        MaxWaitTime time.Duration `json:"maxWaitTime" yaml:"maxWaitTime"`
    } `json:"anchorTransmission" yaml:"anchorTransmission"`
}
```

#### 2.3 Node Initialization

**File to modify:** [cmd/accumulate/run.go](cmd/accumulate/run.go)

Initialize the anchor transmission components during node startup:

```go
// Add to accumulate/cmd/accumulate/run.go

func initAnchorTransmission(cmd *cobra.Command, args []string) error {
    // Get the node context
    ctx := cmd.Context().(*node.Context)
    
    // Check if deterministic anchor transmission is enabled
    if !ctx.Config.AnchorTransmission.Enabled {
        return nil
    }
    
    // Create logger
    logger := ctx.Logger.With("module", "anchor-transmission")
    
    // Create generator
    generator := NewDeterministicAnchorGenerator(
        ctx.Database,
        ctx.Config.Network.ID,
        ctx.Config.Partition.ID,
        logger,
    )
    
    // Create transaction builder
    txBuilder := NewDeterministicTxBuilder(logger)
    
    // Get validator key
    validatorKey := ctx.Config.Validator.Key
    
    // Create signature manager
    sigManager := NewSignatureManager(
        validatorKey,
        protocol.PartitionUrl(ctx.Config.Partition.ID).JoinPath(protocol.Validator),
        ctx.Config.AnchorTransmission.RpcEndpoint,
        logger,
    )
    
    // Create transaction monitor
    txMonitor := NewTransactionMonitor(
        ctx.Config.AnchorTransmission.RpcEndpoint,
        logger,
    )
    
    // Create orchestrator
    orchestrator := NewAnchorOrchestrator(
        generator,
        txBuilder,
        sigManager,
        txMonitor,
        ctx.Config.Network,
        ctx.Config.Partition.ID,
        logger,
    )
    
    // Store the orchestrator in the context
    ctx.AnchorOrchestrator = orchestrator
    
    return nil
}
```

### 3. Consensus and Validation Rules

#### 3.1 Transaction Validation

**File to modify:** [internal/core/execute/v2/chain/anchor.go](internal/core/execute/v2/chain/anchor.go)

Update the transaction validation rules to handle deterministic anchor transactions:

```go
// Add to accumulate/internal/core/execute/v2/chain/anchor.go

// ValidateAnchorTransaction validates an anchor transaction
func ValidateAnchorTransaction(ctx context.Context, batch *database.Batch, tx *protocol.Transaction) error {
    // Existing validation logic
    
    // Additional validation for deterministic anchor transactions
    anchor, ok := tx.Body.(*protocol.PartitionAnchor)
    if !ok {
        return errors.New("transaction body is not a partition anchor")
    }
    
    // Verify the transaction is properly constructed
    // This ensures that validators follow the deterministic construction rules
    
    // Check if the initiator is correctly set
    txHash := sha256.Sum256(append(anchor.StateTreeAnchor[:], []byte(fmt.Sprintf("%d", anchor.RootChainIndex))...))
    expectedInitiator := url.URL{
        Authority: protocol.TxSyntheticId,
        Path:      fmt.Sprintf("/%x", txHash[:8]),
    }
    
    if !tx.Header.Initiator.Equal(&expectedInitiator) {
        return errors.New("invalid initiator for deterministic anchor transaction")
    }
    
    // Continue with existing validation
    return nil
}
```

#### 3.2 Signature Aggregation

**File to modify:** [internal/core/execute/v2/chain/signature.go](internal/core/execute/v2/chain/signature.go)

Implement signature aggregation to optimize performance:

```go
// Add to accumulate/internal/core/execute/v2/chain/signature.go

// ProcessSignatureForDeterministicAnchor processes a signature for a deterministic anchor
func ProcessSignatureForDeterministicAnchor(ctx context.Context, batch *database.Batch, sig *protocol.Signature, txHash [32]byte) error {
    // Get the transaction
    tx, err := batch.Transaction(txHash).Get()
    if err != nil {
        if errors.Is(err, errors.NotFound) {
            // Transaction not found, store the signature for later
            return StoreOrphanedSignature(batch, sig, txHash)
        }
        return err
    }
    
    // Check if this is a partition anchor
    anchor, ok := tx.Body.(*protocol.PartitionAnchor)
    if !ok {
        // Not a partition anchor, process normally
        return ProcessSignatureNormally(ctx, batch, sig, txHash, tx)
    }
    
    // Verify the signature
    err = VerifySignature(sig, txHash[:])
    if err != nil {
        return err
    }
    
    // Add the signature to the transaction
    err = batch.Transaction(txHash).AddSignature(sig)
    if err != nil {
        return err
    }
    
    // Check if we have enough signatures to execute
    // For deterministic anchors, we need a threshold of signatures from validators
    threshold, err := CalculateValidatorThreshold(ctx, batch, anchor.Source)
    if err != nil {
        return err
    }
    
    // Count valid signatures
    sigCount, err := CountValidSignatures(ctx, batch, txHash)
    if err != nil {
        return err
    }
    
    // If we have enough signatures, execute the transaction
    if sigCount >= threshold {
        return ExecuteTransaction(ctx, batch, tx)
    }
    
    // Not enough signatures yet
    return nil
}
```

## Migration Plan

### 1. Code Changes Required

To migrate from the current implementation to the deterministic anchor transmission system, the following code changes are required:

#### 1.1 New Components

1. Implement the core components described in the detailed design:
   - `DeterministicAnchorGenerator`
   - `DeterministicTxBuilder`
   - `SignatureManager`
   - `TransactionMonitor`
   - `AnchorOrchestrator`

2. Update the ABCI application to integrate with the new system:
   - Modify `EndBlock` to trigger anchor transmission
   - Add configuration options for the new system

3. Update transaction validation to handle deterministic anchor transactions:
   - Add validation rules for deterministic transaction construction
   - Implement signature aggregation for deterministic anchors

#### 1.2 Modified Components

1. Update the dispatcher to handle deterministic anchor transactions:
   - Modify `Submit` to recognize and prioritize deterministic anchors
   - Update routing logic to handle signature aggregation

2. Modify the transaction executor:
   - Update anchor transaction execution to work with deterministic anchors
   - Implement threshold signature verification for validators

3. Update the database schema:
   - Add indexes for tracking deterministic anchor transactions
   - Add fields for tracking signature thresholds and counts

#### 1.3 Removed Components

1. Remove the current leader-based anchor transmission:
   - Remove `Conductor.sendAnchors` method
   - Remove leader election logic for anchor transmission

2. Remove or simplify healing mechanisms:
   - Simplify `Conductor.healAnchors` to focus on edge cases
   - Remove complex detection logic for missing anchors

3. Remove single-leader submission model:
   - Remove leader-specific code paths for anchor submission
   - Remove leader-specific error handling

### 2. Migration Steps

#### Phase 1: Development and Testing (2 weeks)

1. **Week 1: Core Implementation**
   - Implement all new components
   - Update existing components to work with deterministic anchors
   - Create unit tests for all new functionality

2. **Week 2: Integration and Testing**
   - Integrate new components with existing codebase
   - Create integration tests for the full system
   - Test with multiple validators in a controlled environment

#### Phase 2: Deployment (2 weeks)

1. **Week 1: Testnet Deployment**
   - Deploy to testnet with feature flag enabled
   - Run both systems in parallel for comparison
   - Monitor performance and reliability metrics
   - Address any issues identified during testing

2. **Week 2: Mainnet Preparation and Rollout**
   - Finalize code based on testnet feedback
   - Prepare documentation and release notes
   - Deploy to mainnet with feature flag enabled
   - Gradually enable for all validators
   - Monitor system performance and reliability

#### Phase 3: Cleanup (1 week)

1. **Days 1-3: Monitoring and Verification**
   - Ensure all anchors are being properly transmitted
   - Verify that no healing is needed for new anchors
   - Confirm performance improvements

2. **Days 4-7: Code Cleanup**
   - Remove old code paths once new system is stable
   - Remove feature flags and conditional logic
   - Update documentation to reflect the new system

### 3. Backward Compatibility

During the migration, both systems will operate in parallel to ensure backward compatibility:

1. **Feature Flag**: A feature flag will control which system is active for each validator
2. **Dual Processing**: Validators with the new system will process anchors deterministically while still participating in the old system
3. **Gradual Transition**: As more validators upgrade, the system will naturally transition to the new approach
4. **Fallback Mechanism**: If issues are detected, validators can fall back to the old system by disabling the feature flag

### 4. Rollback Plan

In case of critical issues, a rollback plan is essential:

1. **Immediate Disable**: Feature flag can be disabled immediately if issues are detected
2. **Automatic Fallback**: If deterministic transactions fail, the system falls back to leader-based submission
3. **Monitoring Alerts**: Set up alerts to detect issues with anchor transmission
4. **Emergency Patches**: Prepare emergency patches for critical issues

## Conclusion

The deterministic anchor transmission system offers a significant improvement over the current leader-based approach. By enabling all validators to independently generate and sign identical transactions, it eliminates single points of failure and improves reliability.

The migration plan provides a structured approach to implementing this new system while maintaining backward compatibility and minimizing risks. With a total implementation time of approximately 5 weeks, this approach offers a relatively quick path to improved reliability compared to more complex solutions like IPC.

## References

1. [Accumulate Cross-Network Communication](../implementation/02_cross_network_communication.md)
2. [Anchor Proofs and Receipts](../implementation/03_anchor_proofs_and_receipts.md)
3. [CometBFT P2P Networking](06_cometbft_p2p_networking.md)
4. [Deterministic Anchor Transmission Overview](12_deterministic_anchor_transmission.md)
