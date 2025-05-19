# Reporting System

## Introduction

The reporting system is a critical component of the healing processes in Accumulate. It provides visibility into the healing progress, helps identify issues, and allows operators to monitor the health of the network. This document details the reporting system's implementation, features, and best practices.

## Reporting Architecture

### Report Types

The healing processes include several types of reports:

1. **Progress Reports**: Real-time updates on healing progress
2. **Summary Reports**: Overall statistics at the end of the healing process
3. **Detailed Reports**: In-depth information about specific transactions or anchors
4. **Error Reports**: Information about failures and issues

### Report Formatting

Reports are formatted using ASCII characters for better terminal compatibility:

```go
// From tools/cmd/debug/heal_common.go
func (h *healer) printDetailedReport() {
    // Print a header for the report with distinctive formatting
    fmt.Println()
    fmt.Println("+-----------------------------------------------------------+")
    fmt.Println("|                HEALING DETAILED REPORT                     |")
    fmt.Println("+-----------------------------------------------------------+")
    
    // Calculate and print the total runtime
    runtime := time.Since(h.startTime)
    fmt.Print("* Total Runtime: ")
    fmt.Println(runtime.Round(time.Second))
    
    // Print delivery statistics
    fmt.Println("\n* DELIVERY STATUS:")
    fmt.Printf("   Transactions Submitted: %d\n", h.txSubmitted)
    fmt.Printf("   Transactions Delivered: %d\n", h.txDelivered)
    fmt.Printf("   Transactions Failed: %d\n", h.totalFailed)
    
    // Print partition pair statistics
    fmt.Println("\n* PARTITION PAIR STATISTICS:")
    for _, pairStat := range h.pairStats {
        fmt.Printf("   %s -> %s:\n", pairStat.Source, pairStat.Destination)
        fmt.Printf("      Submitted: %d, Delivered: %d, Failed: %d\n", 
            pairStat.TxSubmitted, pairStat.TxDelivered, pairStat.TxFailed)
        fmt.Printf("      Current Delivered: %d\n", pairStat.CurrentDelivered)
        fmt.Printf("      Last Updated: %s\n", 
            pairStat.LastUpdated.Format("2006-01-02 15:04:05"))
        fmt.Printf("      Up To Date: %v\n", pairStat.IsUpToDate)
    }
}
```

This ASCII formatting ensures that reports are readable in any terminal environment.

## Progress Reporting

### Real-Time Updates

The healing processes provide real-time updates on progress:

```go
// From tools/cmd/debug/heal_synth.go
slog.InfoContext(h.ctx, "Healed transaction", 
    "source", source, "destination", destination, "number", number, "id", id,
    "submitted", h.txSubmitted, "delivered", h.txDelivered)
```

These updates include information about the specific transaction or anchor being healed, as well as overall progress statistics.

### Periodic Reports

The healing processes include periodic reporting to provide regular updates without overwhelming the operator:

```go
// From tools/cmd/debug/heal_common.go
// Set up periodic reporting
if reportInterval > 0 {
    ticker := time.NewTicker(reportInterval)
    defer ticker.Stop()
    
    go func() {
        for {
            select {
            case <-ticker.C:
                h.printDetailedReport()
            case <-h.ctx.Done():
                return
            }
        }
    }()
}
```

This periodic reporting allows operators to monitor progress without having to manually request updates.

## Partition Pair Statistics

### Tracking Partition Pairs

The healing processes track statistics for each partition pair:

```go
// From tools/cmd/debug/heal_common.go
type PartitionPairStats struct {
    Source           string    // Source partition ID
    Destination      string    // Destination partition ID
    TxSubmitted      int       // Number of transactions submitted for this pair
    TxDelivered      int       // Number of transactions delivered for this pair
    TxFailed         int       // Number of transactions failed for this pair
    CurrentDelivered uint64    // Current sequence number delivered
    LastUpdated      time.Time // Last time this pair was updated
    IsUpToDate       bool      // Whether this pair is up to date
}
```

These statistics provide detailed information about the healing progress for each partition pair.

### Updating Partition Pair Statistics

The healing processes update partition pair statistics as transactions or anchors are healed:

```go
// From tools/cmd/debug/heal_synth.go
func (h *healer) healSingleSynth(source, destination string, number uint64, id *url.TxID, txns map[[32]byte]*protocol.Transaction) bool {
    // ...
    
    // Update pair statistics
    pairKey := fmt.Sprintf("%s:%s", source, destination)
    if pairStat, ok := h.pairStats[pairKey]; ok {
        pairStat.TxSubmitted++
        pairStat.LastUpdated = time.Now()
    }
    
    // ...
    
    // Successfully healed the transaction
    h.txDelivered++
    h.totalHealed++
    
    // Update pair statistics
    if pairStat, ok := h.pairStats[pairKey]; ok {
        pairStat.TxDelivered++
        pairStat.CurrentDelivered = number
        pairStat.LastUpdated = time.Now()
    }
    
    // ...
}
```

This updating ensures that the partition pair statistics are always current and accurate.

## Summary Reporting

### Final Report

The healing processes generate a final report at the end of the healing process:

```go
// From tools/cmd/debug/heal_synth.go
func healSynth(cmd *cobra.Command, args []string) {
    // ...
    
    // Run the healing process
    h.heal(args)
    
    // Print a final report
    h.printDetailedReport()
    
    // ...
}
```

This final report provides a comprehensive summary of the healing process, including overall statistics and partition pair statistics.

### Success and Failure Counts

The final report includes counts of successful and failed healing operations:

```go
// From tools/cmd/debug/heal_common.go
func (h *healer) printDetailedReport() {
    // ...
    
    // Print delivery statistics
    fmt.Println("\n* DELIVERY STATUS:")
    fmt.Printf("   Transactions Submitted: %d\n", h.txSubmitted)
    fmt.Printf("   Transactions Delivered: %d\n", h.txDelivered)
    fmt.Printf("   Transactions Failed: %d\n", h.totalFailed)
    
    // ...
}
```

These counts help operators understand the overall success rate of the healing process.

## Error Reporting

### Error Logging

The healing processes include detailed error logging:

```go
// From tools/cmd/debug/heal_synth.go
if errors.Is(err, errors.Delivered) {
    // Transaction already delivered
    h.txDelivered++
    
    // Update pair statistics
    if pairStat, ok := h.pairStats[pairKey]; ok {
        pairStat.TxDelivered++
        pairStat.CurrentDelivered = number
        pairStat.LastUpdated = time.Now()
    }
    
    slog.InfoContext(h.ctx, "Transaction already delivered", 
        "source", source, "destination", destination, "number", number, "id", id,
        "submitted", h.txSubmitted, "delivered", h.txDelivered)
    return false
} else if errors.Is(err, errors.Pending) {
    // Transaction pending
    slog.InfoContext(h.ctx, "Transaction pending", 
        "source", source, "destination", destination, "number", number, "id", id,
        "submitted", h.txSubmitted, "delivered", h.txDelivered)
    return true
} else {
    // Other error
    h.totalFailed++
    
    // Update pair statistics
    if pairStat, ok := h.pairStats[pairKey]; ok {
        pairStat.TxFailed++
        pairStat.LastUpdated = time.Now()
    }
    
    slog.ErrorContext(h.ctx, "Failed to heal transaction", 
        "source", source, "destination", destination, "number", number, "id", id,
        "error", err, "submitted", h.txSubmitted, "delivered", h.txDelivered)
    return false
}
```

This error logging provides detailed information about failures, including the specific error and the transaction or anchor that failed.

### Error Classification

The healing processes classify errors to provide more meaningful reports:

```go
// From tools/cmd/debug/heal_common.go
if errors.Is(err, errors.Delivered) {
    // Transaction already delivered
    // ...
} else if errors.Is(err, errors.Pending) {
    // Transaction pending
    // ...
} else if errors.Is(err, errors.NotFound) {
    // Transaction not found
    // ...
} else {
    // Other error
    // ...
}
```

This classification helps operators understand the types of errors that are occurring and take appropriate action.

## Command-Line Interface Integration

### Flags for Reporting

The command-line interface includes flags for controlling reporting behavior:

```go
// From tools/cmd/debug/heal_synth.go
func init() {
    // ...
    
    healSynthCmd.Flags().DurationVar(&reportInterval, "report-interval", 5*time.Minute, "Interval for printing detailed reports")
    
    // ...
}
```

These flags allow operators to customize the reporting behavior to meet their needs.

### Interactive Reports

The command-line interface supports interactive reporting:

```go
// From tools/cmd/debug/heal_common.go
func (h *healer) printDetailedReport() {
    // ...
    
    // Print partition pair statistics
    fmt.Println("\n* PARTITION PAIR STATISTICS:")
    for _, pairStat := range h.pairStats {
        fmt.Printf("   %s -> %s:\n", pairStat.Source, pairStat.Destination)
        fmt.Printf("      Submitted: %d, Delivered: %d, Failed: %d\n", 
            pairStat.TxSubmitted, pairStat.TxDelivered, pairStat.TxFailed)
        fmt.Printf("      Current Delivered: %d\n", pairStat.CurrentDelivered)
        fmt.Printf("      Last Updated: %s\n", 
            pairStat.LastUpdated.Format("2006-01-02 15:04:05"))
        fmt.Printf("      Up To Date: %v\n", pairStat.IsUpToDate)
    }
}
```

These interactive reports allow operators to view detailed information about the healing process in real-time.

## Report Customization

### Timestamp Formatting

The healing processes include customizable timestamp formatting:

```go
// From tools/cmd/debug/heal_common.go
fmt.Printf("      Last Updated: %s\n", 
    pairStat.LastUpdated.Format("2006-01-02 15:04:05"))
```

This formatting ensures that timestamps are readable and consistent.

### Color Coding

The healing processes include color coding for better readability:

```go
// From tools/cmd/debug/heal_anchor.go
func (h *healer) printAnchorHealingReport() {
    // Print a header for the report
    fmt.Println()
    color.New(color.FgHiCyan).Add(color.Bold).Println("+-----------------------------------------------------------+")
    color.New(color.FgHiCyan).Add(color.Bold).Println("|                ANCHOR HEALING REPORT                      |")
    color.New(color.FgHiCyan).Add(color.Bold).Println("+-----------------------------------------------------------+")
    
    // Calculate and print the total runtime
    runtime := time.Since(h.startTime)
    color.New(color.FgHiWhite).Add(color.Bold).Print("* Total Runtime: ")
    color.New(color.FgHiYellow).Println(runtime.Round(time.Second))
    
    // Print delivery statistics
    color.New(color.FgHiWhite).Add(color.Bold).Println("\n* DELIVERY STATUS:")
    color.New(color.FgHiWhite).Print("   Transactions Submitted: ")
    color.New(color.FgHiYellow).Printf("%d\n", h.txSubmitted)
    color.New(color.FgHiWhite).Print("   Transactions Delivered: ")
    color.New(color.FgHiYellow).Printf("%d\n", h.txDelivered)
    color.New(color.FgHiWhite).Print("   Transactions Failed: ")
    color.New(color.FgHiYellow).Printf("%d\n", h.totalFailed)
    
    // ...
}
```

This color coding makes the reports more readable and helps highlight important information.

## Recent Changes

Recent changes to the reporting system include:

1. **ASCII Formatting**: Updated report formatting to use ASCII characters for better terminal compatibility
2. **Timestamp Formatting**: Improved timestamp formatting for readability
3. **Color Coding**: Added color coding for better readability
4. **Partition Pair Tracking**: Enhanced partition pair tracking to provide more detailed information

## Best Practices

When working with the reporting system in the healing processes, it's important to follow these best practices:

1. **Use ASCII Formatting**: Use ASCII formatting for better terminal compatibility
2. **Include Timestamps**: Include timestamps in reports to provide context
3. **Classify Errors**: Classify errors to provide more meaningful reports
4. **Update Partition Pair Statistics**: Keep partition pair statistics up to date
5. **Use Color Coding**: Use color coding for better readability

## Command-Line Flags

The healing processes include several command-line flags for controlling reporting behavior:

```go
// From tools/cmd/debug/heal_synth.go
func init() {
    // ...
    
    healSynthCmd.Flags().DurationVar(&reportInterval, "report-interval", 5*time.Minute, "Interval for printing detailed reports")
    healSynthCmd.Flags().DurationVar(&since, "since", 48*time.Hour, "How far back in time to heal (0 for forever)")
    healSynthCmd.Flags().DurationVar(&maxResponseAge, "max-response-age", 336*time.Hour, "Maximum age of a response")
    healSynthCmd.Flags().BoolVar(&wait, "wait", true, "Wait for transactions")
    
    // ...
}
```

These flags allow operators to customize the healing process and reporting behavior:

- **report-interval**: Controls how often detailed reports are printed
- **since**: Controls how far back in time to heal
- **max-response-age**: Controls the maximum age of a response
- **wait**: Controls whether to wait for transactions to be delivered

## Conclusion

The reporting system is a critical component of the healing processes in Accumulate. By providing visibility into the healing progress, it helps operators monitor the health of the network and identify issues. The reporting system includes features such as real-time updates, periodic reports, partition pair statistics, and error reporting.

In the next document, we will explore the error handling mechanisms used by the healing processes to handle various error conditions.
