# Deterministic Anchor - Part 3: Implementation

This document details the implementation of the deterministic anchor system in Accumulate.

## Core Components

The deterministic anchor system is implemented through several core components:

### Anchor Generator

```go
type AnchorGenerator struct {
    State       *State
    BlockHeight uint64
    BlockTime   time.Time
    ChainID     string
}

func (g *AnchorGenerator) GenerateAnchor() (*protocol.Anchor, error) {
    // Get the current state root
    stateRoot := g.State.GetStateRoot()
    
    // Create the anchor
    anchor := &protocol.Anchor{
        SourceChain:  g.ChainID,
        StateRoot:    stateRoot,
        BlockHeight:  g.BlockHeight,
        Timestamp:    g.BlockTime,
        SequenceNumber: g.State.GetNextAnchorSequence(),
    }
    
    // Determine destination chains
    destinations := g.DetermineDestinations()
    anchor.Destinations = destinations
    
    return anchor, nil
}
```

### Anchor Scheduler

```go
type AnchorScheduler struct {
    NetworkParams *protocol.NetworkParameters
}

func (s *AnchorScheduler) ShouldGenerateAnchor(blockHeight uint64) bool {
    // Check if this block height should generate an anchor
    interval := s.NetworkParams.AnchorInterval
    return blockHeight % interval == 0
}
```

### Anchor Router

```go
type AnchorRouter struct {
    NetworkTopology *protocol.NetworkTopology
}

func (r *AnchorRouter) DetermineDestinations(sourceChain string) []string {
    // Determine destination chains based on network topology
    if r.NetworkTopology.IsBVN(sourceChain) {
        // BVNs anchor to the DN
        return []string{r.NetworkTopology.GetDN()}
    } else if r.NetworkTopology.IsDN(sourceChain) {
        // DN anchors to all BVNs
        return r.NetworkTopology.GetAllBVNs()
    }
    
    return nil
}
```

## Integration with ABCI

The deterministic anchor system integrates with CometBFT through the ABCI interface:

```go
func (app *AccumulateApp) EndBlock(req abci.RequestEndBlock) abci.ResponseEndBlock {
    // Check if we should generate an anchor
    if app.AnchorScheduler.ShouldGenerateAnchor(req.Height) {
        // Generate the anchor
        anchor, err := app.AnchorGenerator.GenerateAnchor()
        if err != nil {
            app.Logger.Error("Failed to generate anchor", zap.Error(err))
            return abci.ResponseEndBlock{}
        }
        
        // Create synthetic transactions for the anchor
        txs := app.CreateAnchorTransactions(anchor)
        
        // Add the transactions to the next block
        app.PendingAnchorTxs = txs
    }
    
    return abci.ResponseEndBlock{}
}
```

## Synthetic Transaction Generation

Anchors are transmitted through synthetic transactions:

```go
func (app *AccumulateApp) CreateAnchorTransactions(anchor *protocol.Anchor) []*protocol.Transaction {
    var txs []*protocol.Transaction
    
    // Create a synthetic transaction for each destination
    for _, dest := range anchor.Destinations {
        tx := &protocol.Transaction{
            Header: &protocol.TransactionHeader{
                Principal: protocol.AnchorLedgerUrl(dest),
            },
            Body: &protocol.TransactionBody{
                Type: protocol.TransactionTypeAnchor,
                Anchor: &protocol.AnchorBody{
                    Source:         anchor.SourceChain,
                    StateRoot:      anchor.StateRoot,
                    BlockHeight:    anchor.BlockHeight,
                    Timestamp:      anchor.Timestamp,
                    SequenceNumber: anchor.SequenceNumber,
                },
            },
        }
        
        txs = append(txs, tx)
    }
    
    return txs
}
```

## Performance Optimizations

Several optimizations improve the performance of the deterministic anchor system:

1. **Batch Processing**: Anchors are processed in batches
2. **Caching**: Frequently accessed data is cached
3. **Parallel Processing**: Independent operations are parallelized
4. **Compression**: Anchor data is compressed to reduce size
