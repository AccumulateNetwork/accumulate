# BlockchainDB Performance Guide

This guide provides information about optimizing performance and understanding the trade-offs in different usage scenarios for BlockchainDB.

## Performance Characteristics

BlockchainDB is designed with specific performance characteristics that make it well-suited for blockchain applications:

1. **Optimized for Write-Once, Read-Many**: The database structure is optimized for scenarios where data is written once and read many times, which is common in blockchain applications.

2. **Efficient Key Lookups**: The key organization strategy in KFile enables efficient key lookups, which is critical for blockchain state verification.

3. **Reduced Disk I/O**: The buffered file implementation significantly reduces disk I/O operations, improving overall performance.

4. **Historical State Access**: When enabled, the history tracking mechanism provides efficient access to historical states, which is essential for blockchain applications.

## Performance Tuning Parameters

Several parameters can be tuned to optimize BlockchainDB performance for specific use cases:

### Buffer Size

The `BufferSize` constant (default: 32KB) determines the size of the buffer used for file I/O operations. Increasing this value can improve performance for write-heavy workloads but increases memory usage.

### Maximum Cached Blocks

The `MaxCachedBlocks` parameter controls how many blocks are cached in memory before being flushed to disk. Higher values improve write performance but increase memory usage.

### Key Limit

The `KeyLimit` parameter determines how many keys are stored in the KFile before being pushed to the history file. Higher values can improve write performance but may increase memory usage during history pushes.

### Offset Count

The `OffsetCnt` parameter affects how keys are organized in the KFile and HistoryFile. Higher values can improve lookup performance but increase memory usage.

## Performance Optimization Strategies

### 1. Tune Memory Usage

Balance memory usage with performance by adjusting the buffer size and maximum cached blocks based on your available memory and workload characteristics.

```go
// Example: Increase buffer size for write-heavy workloads with ample memory
const BufferSize = 1024 * 64 // 64KB instead of 32KB

// Example: Reduce cached blocks for memory-constrained environments
kv, err := blockchainDB.NewKV(
    true,
    "/path/to/db",
    1024,
    10000,
    50, // Reduced from default 100
)
```

### 2. Disable History for Read-Only Use Cases

If historical state access is not required, disabling history tracking can significantly improve performance and reduce storage requirements.

```go
// Example: Create KV without history
kv, err := blockchainDB.NewKV(
    false, // Disable history
    "/path/to/db",
    1024,
    10000,
    100,
)
```

### 3. Batch Operations

When performing multiple operations, batch them together to minimize disk I/O.

```go
// Example: Batch multiple put operations
for _, item := range items {
    kv.Put(item.Key, item.Value)
    // No intermediate flush or close operations
}
// Single close operation after all puts
kv.Close()
```

### 4. Use Compression Strategically

The `Compress()` method can reclaim space from deleted or updated values, but it's an expensive operation. Use it strategically during low-usage periods.

```go
// Example: Compress during off-peak hours
if isOffPeakHours() {
    kv.Compress()
}
```

## Performance Benchmarks

The BlockchainDB test suite includes benchmarks that can be used to measure performance on your specific hardware and with your specific configuration.

```bash
# Run benchmarks
go test -bench=. github.com/AccumulateNetwork/BlockchainDB/database
```

## Common Performance Issues and Solutions

### Issue: Slow Key Lookups

**Possible causes:**
- Inefficient offset count
- Large number of keys in a single section

**Solutions:**
- Increase the offset count to distribute keys more evenly
- Ensure keys are well-distributed across the hash space

### Issue: High Memory Usage

**Possible causes:**
- Too many cached blocks
- Large buffer size

**Solutions:**
- Reduce the maximum cached blocks
- Consider reducing the buffer size if memory constraints are severe

### Issue: Slow Write Performance

**Possible causes:**
- Frequent flushing to disk
- Small buffer size

**Solutions:**
- Increase the buffer size
- Increase the maximum cached blocks
- Batch operations where possible
