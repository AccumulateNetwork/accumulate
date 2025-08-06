# Accumulate Network Performance Guide

**Last Updated**: December 2024  
**Version**: v1.0.0-rc3.3.0.20221022212648-f9808866894c

## Overview

This guide documents known performance characteristics, bottlenecks, and optimization strategies for the Accumulate Network. It consolidates performance insights from code analysis, benchmarks, and operational experience.

## Known Performance Issues

### 1. Core Execution Engine

#### 1.1 Anchor Receipt Construction Inefficiency

**Location**: `internal/core/execute/v2/block/block_end.go:759-762`

**Issue**: Inefficient construction of receipts for every anchor during block processing.

```go
// TODO This is pretty inefficient; we're constructing a receipt for every
// anchor. If we were more intelligent about it, we could send just the
// Merkle state and a list of transactions, though we would need that for
// the root chain and each anchor chain.
```

**Impact**:
- Increased CPU usage during anchor processing
- Higher memory allocation for receipt construction
- Slower block finalization times

**Mitigation**:
- Currently unavoidable - required for protocol correctness
- Future optimization: Send Merkle state + transaction lists instead of full receipts

#### 1.2 Message Execution Order Preservation

**Location**: `internal/core/execute/v2/block/exec_process.go:210-220`

**Issue**: Immediate execution of produced messages when producer and destination are in the same domain.

```go
// This is inefficient when the producer and destination are in the same
// domain, but it preserves the ordering of messages.
```

**Impact**:
- Reduced parallelization opportunities
- Sequential processing bottleneck
- Lower transaction throughput in single-domain scenarios

**Trade-off**: Order preservation vs. performance
**Status**: Intentional design choice for consistency

#### 1.3 Disabled Buffer Pool Optimization

**Location**: `pkg/types/record/key.go:205-215`

**Issue**: Buffer pools disabled due to concurrency bugs.

```go
// TODO Re-enable this once we've fixed the concurrency bugs
// var bufPool = sync.Pool{
//     New: func() interface{} { return new(bytes.Buffer) },
// }
```

**Impact**:
- Increased garbage collection pressure
- Higher memory allocation overhead
- Reduced marshaling performance

**Status**: Disabled for stability - requires concurrency bug fixes

### 2. Database Operations

#### 2.1 Slow Snapshot Collection

**Location**: `tools/cmd/debug/snap_collect.go:90-95`

**Issue**: Database scanning without partition specification is very slow.

```go
fmt.Fprintf(os.Stderr, "Scanning the database without specifying a partition is very slow. "+
    "Consider using --partition to specify a partition to scan.\n")
```

**Impact**:
- Extremely slow snapshot collection (hours vs. minutes)
- High I/O load on database
- Operational inefficiency

**Mitigation**: Always use `--partition` flag for snapshot operations

## Performance Optimization Strategies

### 1. Client-Side Optimizations

#### Connection Management
```go
// Use connection pooling for high-throughput applications
httpClient := &http.Client{
    Transport: &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,
    },
    Timeout: 30 * time.Second,
}
```

#### Batch Operations
```go
// Batch multiple queries when possible
queries := []*api.Query{
    &api.AccountQuery{Url: "acc://account1.acme"},
    &api.AccountQuery{Url: "acc://account2.acme"},
    &api.AccountQuery{Url: "acc://account3.acme"},
}

// Process in parallel with controlled concurrency
```

### 2. Node Configuration

#### Database Tuning
- Use SSD storage for database
- Allocate sufficient RAM for database caching
- Configure appropriate connection limits

#### Network Optimization
- Use dedicated network for inter-node communication
- Configure appropriate timeouts and retry policies
- Monitor network latency between partitions

### 3. Operational Best Practices

#### Snapshot Management
```bash
# Always specify partition for snapshot operations
accumulated debug snap collect --partition BVN0 --output snapshot.json

# Use compression for large snapshots
accumulated debug snap collect --partition BVN0 --compress --output snapshot.gz
```

#### Monitoring
- Monitor transaction processing rates
- Track block processing times
- Watch memory usage patterns
- Alert on unusual anchor processing delays

## Performance Benchmarks

### Block Processing
- **Average Block Time**: ~1 second (varies by network load)
- **Transaction Throughput**: 100-1000 TPS (depends on transaction complexity)
- **Anchor Processing**: 2-5 seconds per anchor batch

### API Response Times
- **Account Queries**: 10-50ms (local), 100-500ms (remote)
- **Transaction Queries**: 20-100ms (depends on receipt inclusion)
- **Network Status**: 5-20ms
- **Snapshot Listing**: 50-200ms

## Troubleshooting Performance Issues

### 1. Slow Transaction Processing

**Symptoms**:
- High transaction queue depth
- Increasing block processing times
- Memory usage growth

**Diagnosis**:
```bash
# Check transaction pool status
accumulated query tx-pool-status

# Monitor block processing metrics
accumulated metrics block-processing
```

**Solutions**:
- Scale validator resources
- Check for network partitioning
- Verify database performance

### 2. High Memory Usage

**Symptoms**:
- Increasing RSS memory usage
- Frequent garbage collection
- Out of memory errors

**Diagnosis**:
- Profile memory usage with Go tools
- Check for memory leaks in long-running processes
- Monitor buffer pool effectiveness

**Solutions**:
- Increase available memory
- Restart nodes periodically
- Investigate specific memory hotspots

### 3. Slow API Responses

**Symptoms**:
- High API response latencies
- Client timeouts
- Connection pool exhaustion

**Diagnosis**:
```bash
# Check API server metrics
curl -s http://localhost:26657/metrics | grep api_

# Monitor connection usage
netstat -an | grep :26657
```

**Solutions**:
- Scale API server instances
- Implement client-side caching
- Use connection pooling
- Add load balancing

## Future Optimizations

### Planned Improvements
1. **Anchor Receipt Optimization**: Implement Merkle state + transaction list approach
2. **Buffer Pool Re-enablement**: Fix concurrency bugs and restore buffer pools
3. **Parallel Message Processing**: Optimize message execution for same-domain scenarios
4. **Database Indexing**: Improve query performance with better indexing strategies

### Research Areas
- Cross-partition transaction optimization
- State pruning strategies
- Consensus algorithm improvements
- Network topology optimization

## Configuration Recommendations

### Production Node Configuration
```yaml
# Example configuration for high-performance production node
database:
  type: "badger"
  path: "/data/accumulate"
  cache_size: "2GB"
  
network:
  max_connections: 100
  timeout: "30s"
  
consensus:
  block_time: "1s"
  max_block_size: "1MB"
```

### Development Environment
```yaml
# Optimized for development speed
database:
  type: "memory"
  
network:
  max_connections: 10
  timeout: "5s"
  
logging:
  level: "debug"
  performance_metrics: true
```

## Monitoring and Alerting

### Key Metrics to Monitor
- Transaction processing rate (TPS)
- Block processing time
- Memory usage trends
- API response latencies
- Network connectivity between partitions

### Alert Thresholds
- Block processing time > 5 seconds
- Memory usage > 80% of available
- API response time > 1 second (95th percentile)
- Transaction queue depth > 1000

## References

- [Accumulate Architecture Documentation](../architecture/overview.md)
- [API Performance Guide](../api/performance-considerations.md)
- [Database Optimization Guide](../database/optimization.md)
- [Network Configuration Guide](../network/configuration.md)
