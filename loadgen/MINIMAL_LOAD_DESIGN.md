# Minimal Load Generation Design

## Overview
Single-threaded load generator that submits transactions and waits for results. Tracks transaction counts by type and partition.

## Core Structure

```go
type LoadGenerator struct {
    wallet  *Wallet
    client  Client
    metrics *Metrics
}

type Wallet struct {
    ADIs         []string  // List of ADI URLs
    Accounts     []string  // List of token account URLs
}

type Metrics struct {
    StartTime    time.Time
    TypeCounts   map[string]uint64
    PartitionTx  map[string]uint64  // Transactions per partition
    CrossPartTx  uint64              // Cross-partition count
}
```

## Main Loop

```go
func (lg *LoadGenerator) Run(duration time.Duration) {
    endTime := time.Now().Add(duration)
    
    for time.Now().Before(endTime) {
        // Pick transaction type
        txType := lg.selectTransaction()
        
        // Build and submit
        tx := lg.buildTransaction(txType)
        result := lg.submitAndWait(tx)
        
        // Track metrics
        lg.metrics.TypeCounts[txType]++
        lg.trackPartition(tx, result)
    }
}
```

## Transaction Selection

```go
func (lg *LoadGenerator) selectTransaction() string {
    r := rand.Float64()
    
    // Need some ADIs first
    if len(lg.wallet.ADIs) < 10 {
        return "create_adi"
    }
    
    // Need some accounts
    if len(lg.wallet.Accounts) < 20 {
        return "create_token_account"
    }
    
    // Otherwise send tokens
    return "send_tokens"
}
```

## Partition Tracking

```go
func (lg *LoadGenerator) trackPartition(tx Transaction, result Result) {
    sourcePartition := getPartition(tx.Source)
    destPartition := getPartition(tx.Destination)
    
    lg.metrics.PartitionTx[sourcePartition]++
    
    if sourcePartition != destPartition {
        lg.metrics.CrossPartTx++
    }
}

func getPartition(url string) string {
    // Extract partition from URL
    // e.g., "acc://bvn-0.acme/..." -> "bvn-0"
}
```

## Submit and Wait

```go
func (lg *LoadGenerator) submitAndWait(tx Transaction) Result {
    // Submit
    txID, err := lg.client.Submit(tx)
    if err != nil {
        return Result{Success: false}
    }
    
    // Wait for result
    for {
        status := lg.client.GetStatus(txID)
        if status.Complete {
            return Result{Success: status.Success}
        }
        time.Sleep(100 * time.Millisecond)
    }
}
```

## Final Report

```go
func (lg *LoadGenerator) Report() {
    elapsed := time.Since(lg.metrics.StartTime)
    total := uint64(0)
    
    fmt.Println("Transaction Counts:")
    for txType, count := range lg.metrics.TypeCounts {
        fmt.Printf("  %s: %d\n", txType, count)
        total += count
    }
    
    fmt.Println("\nPartition Distribution:")
    for partition, count := range lg.metrics.PartitionTx {
        fmt.Printf("  %s: %d\n", partition, count)
    }
    
    fmt.Printf("\nCross-Partition: %d (%.1f%%)\n", 
        lg.metrics.CrossPartTx, 
        float64(lg.metrics.CrossPartTx)*100/float64(total))
    
    fmt.Printf("Total: %d transactions in %v\n", total, elapsed)
    fmt.Printf("Average TPS: %.2f\n", float64(total)/elapsed.Seconds())
}
```

## Configuration

```yaml
duration: 60s
initial_adis: 10
initial_accounts: 20
```

## Complete Example

```go
func main() {
    lg := &LoadGenerator{
        wallet:  &Wallet{},
        client:  NewClient(),
        metrics: &Metrics{
            StartTime:   time.Now(),
            TypeCounts:  make(map[string]uint64),
            PartitionTx: make(map[string]uint64),
        },
    }
    
    lg.Run(60 * time.Second)
    lg.Report()
}
```

## Summary

- **~100 lines total** for core functionality
- **Single-threaded** - transactions naturally rate limit
- **Simple metrics** - counts by type and partition
- **Cross-partition tracking** - simple counter
- **No complex features** - just submit, wait, count
- **Transaction layer handles** all actual transaction building