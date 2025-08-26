# Performance Testing Guide

## Table of Contents

1. [Overview](#overview)
2. [Quick Start](#quick-start)
3. [Load Testing](#load-testing)
4. [Benchmark Testing](#benchmark-testing)
5. [Profiling](#profiling)
6. [Stress Testing](#stress-testing)
7. [Performance Metrics](#performance-metrics)
8. [Test Scenarios](#test-scenarios)
9. [Analysis and Optimization](#analysis-and-optimization)
10. [CI/CD Integration](#cicd-integration)
11. [Best Practices](#best-practices)

## Overview

Performance testing ensures the Accumulate Network can handle expected loads and identifies bottlenecks before they impact production. This guide covers load testing, benchmarking, profiling, and performance analysis techniques.

### Performance Testing Types

- **Load Testing**: Normal expected load validation
- **Stress Testing**: Beyond normal capacity testing
- **Spike Testing**: Sudden load increase handling
- **Volume Testing**: Large data set processing
- **Endurance Testing**: Extended period stability
- **Benchmark Testing**: Component-level performance

### Key Performance Metrics

```
Throughput Metrics:
├── Transactions per second (TPS)
├── API requests per second (RPS)
├── Block processing rate
└── Network message rate

Latency Metrics:
├── Transaction confirmation time
├── API response time
├── Block creation time
└── Cross-partition message time

Resource Metrics:
├── CPU utilization
├── Memory usage
├── Disk I/O
└── Network bandwidth
```

## Quick Start

### Prerequisites

```bash
# Verify system resources
free -h          # Check available memory
df -h           # Check disk space
nproc           # Check CPU cores

# Install performance tools
go install github.com/pkg/profile@latest
go install golang.org/x/perf/cmd/benchstat@latest
```

### Run Basic Performance Tests

```bash
# Load testing utility
go run ./test/cmd/load/main.go

# Benchmark tests
go test -bench=. ./...

# CPU profiling
go test -bench=. -cpuprofile=cpu.prof ./pkg/database/values

# Memory profiling
go test -bench=. -memprofile=mem.prof ./internal/api/v3
```

## Load Testing

### Load Testing Utility

The main load testing tool is located at `test/cmd/load/main.go`:

```bash
# Basic load test
go run ./test/cmd/load/main.go \
  -server=http://localhost:8080 \
  -transactions=1000 \
  -duration=60

# High-throughput test
go run ./test/cmd/load/main.go \
  -server=http://localhost:8080 \
  -transactions=10000 \
  -duration=300 \
  -max-goroutines=100

# Custom configuration
go run ./test/cmd/load/main.go \
  -server=http://localhost:8080 \
  -transactions=5000 \
  -duration=120 \
  -max-goroutines=50 \
  -rate-limit=100
```

### Load Test Parameters

```bash
# Core parameters
-server=URL              # Target server URL
-transactions=N          # Total transactions to send
-duration=SECONDS        # Test duration in seconds
-max-goroutines=N        # Maximum concurrent goroutines

# Advanced parameters
-rate-limit=N            # Requests per second limit
-timeout=DURATION        # Request timeout
-key-file=PATH           # Private key file
-account=URL             # Source account URL

# Output options
-verbose                 # Verbose output
-output=FILE             # Results output file
-format=json|csv         # Output format
```

### Load Test Scenarios

#### 1. Baseline Load Test

```bash
# Establish baseline performance
go run ./test/cmd/load/main.go \
  -server=http://localhost:8080 \
  -transactions=1000 \
  -duration=60 \
  -max-goroutines=10 \
  -output=baseline.json
```

#### 2. Capacity Test

```bash
# Find maximum capacity
go run ./test/cmd/load/main.go \
  -server=http://localhost:8080 \
  -transactions=50000 \
  -duration=600 \
  -max-goroutines=200 \
  -rate-limit=1000
```

#### 3. Sustained Load Test

```bash
# Test sustained performance
go run ./test/cmd/load/main.go \
  -server=http://localhost:8080 \
  -transactions=100000 \
  -duration=1800 \
  -max-goroutines=50 \
  -rate-limit=500
```

### Custom Load Test Implementation

```go
package main

import (
    "context"
    "fmt"
    "sync"
    "time"
)

type LoadTester struct {
    client     *APIClient
    config     LoadConfig
    metrics    *Metrics
}

type LoadConfig struct {
    ServerURL      string
    Transactions   int
    Duration       time.Duration
    MaxGoroutines  int
    RateLimit      int
}

func (lt *LoadTester) Run(ctx context.Context) error {
    // Create worker pool
    workers := make(chan struct{}, lt.config.MaxGoroutines)
    var wg sync.WaitGroup
    
    // Rate limiter
    ticker := time.NewTicker(time.Second / time.Duration(lt.config.RateLimit))
    defer ticker.Stop()
    
    // Start workers
    for i := 0; i < lt.config.Transactions; i++ {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            workers <- struct{}{}
            wg.Add(1)
            
            go func() {
                defer wg.Done()
                defer func() { <-workers }()
                
                start := time.Now()
                err := lt.sendTransaction()
                duration := time.Since(start)
                
                lt.metrics.Record(duration, err)
            }()
        }
    }
    
    wg.Wait()
    return nil
}

func (lt *LoadTester) sendTransaction() error {
    // Implement transaction sending logic
    return nil
}
```

## Benchmark Testing

### Running Benchmarks

```bash
# All benchmarks
go test -bench=. ./...

# Specific package benchmarks
go test -bench=. ./pkg/database/values
go test -bench=. ./internal/api/v3

# Specific benchmark
go test -bench=BenchmarkNotFound ./pkg/database/values

# With memory stats
go test -bench=. -benchmem ./...

# Multiple runs for stability
go test -bench=. -count=5 ./...

# Benchmark time control
go test -bench=. -benchtime=10s ./...
go test -bench=. -benchtime=1000x ./...
```

### Writing Benchmarks

#### Basic Benchmark

```go
func BenchmarkValidateURL(b *testing.B) {
    url := "acc://test.acme/account"
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        ValidateURL(url)
    }
}
```

#### Benchmark with Setup

```go
func BenchmarkDatabaseQuery(b *testing.B) {
    // Setup
    db := setupTestDatabase(b)
    defer db.Close()
    
    key := "test-key"
    value := []byte("test-value")
    db.Put(key, value)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := db.Get(key)
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

#### Memory Allocation Benchmark

```go
func BenchmarkCreateAccount(b *testing.B) {
    b.ReportAllocs()
    
    for i := 0; i < b.N; i++ {
        account := &Account{
            URL:     "acc://test.acme/account",
            Balance: 0,
        }
        _ = account
    }
}
```

#### Parallel Benchmark

```go
func BenchmarkConcurrentAccess(b *testing.B) {
    cache := NewCache()
    
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            cache.Get("key")
        }
    })
}
```

### Benchmark Comparison

```bash
# Save baseline
go test -bench=. ./... > baseline.bench

# Make changes and compare
go test -bench=. ./... > optimized.bench

# Compare results
benchstat baseline.bench optimized.bench
```

## Profiling

### CPU Profiling

```bash
# Generate CPU profile
go test -bench=. -cpuprofile=cpu.prof ./pkg/database/values

# Analyze profile
go tool pprof cpu.prof

# Interactive analysis
(pprof) top10
(pprof) list FunctionName
(pprof) web
(pprof) svg > profile.svg
```

### Memory Profiling

```bash
# Generate memory profile
go test -bench=. -memprofile=mem.prof ./internal/api/v3

# Analyze memory usage
go tool pprof mem.prof

# Memory analysis commands
(pprof) top10
(pprof) list FunctionName
(pprof) svg > memory.svg
```

### Trace Analysis

```bash
# Generate execution trace
go test -bench=. -trace=trace.out ./...

# Analyze trace
go tool trace trace.out
```

### Profiling in Code

```go
import (
    "os"
    "runtime/pprof"
)

func BenchmarkWithProfiling(b *testing.B) {
    // CPU profiling
    cpuFile, err := os.Create("cpu.prof")
    if err != nil {
        b.Fatal(err)
    }
    defer cpuFile.Close()
    
    pprof.StartCPUProfile(cpuFile)
    defer pprof.StopCPUProfile()
    
    // Memory profiling
    defer func() {
        memFile, err := os.Create("mem.prof")
        if err != nil {
            b.Fatal(err)
        }
        defer memFile.Close()
        
        pprof.WriteHeapProfile(memFile)
    }()
    
    // Benchmark code
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        // Test implementation
    }
}
```

## Stress Testing

### Network Stress Testing

```bash
# High-concurrency stress test
go run ./test/cmd/load/main.go \
  -server=http://localhost:8080 \
  -transactions=100000 \
  -duration=1800 \
  -max-goroutines=500 \
  -rate-limit=2000
```

### Memory Stress Testing

```go
func TestMemoryStress(t *testing.T) {
    const (
        numGoroutines = 100
        iterations    = 1000
    )
    
    var wg sync.WaitGroup
    
    for i := 0; i < numGoroutines; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            
            for j := 0; j < iterations; j++ {
                // Allocate and process data
                data := make([]byte, 1024*1024) // 1MB
                processData(data)
            }
        }()
    }
    
    wg.Wait()
}
```

### Database Stress Testing

```go
func TestDatabaseStress(t *testing.T) {
    db := setupTestDatabase(t)
    defer db.Close()
    
    const (
        numWriters = 50
        numReaders = 100
        operations = 10000
    )
    
    var wg sync.WaitGroup
    
    // Writers
    for i := 0; i < numWriters; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            
            for j := 0; j < operations; j++ {
                key := fmt.Sprintf("key-%d-%d", id, j)
                value := []byte(fmt.Sprintf("value-%d-%d", id, j))
                
                err := db.Put(key, value)
                require.NoError(t, err)
            }
        }(i)
    }
    
    // Readers
    for i := 0; i < numReaders; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            
            for j := 0; j < operations; j++ {
                key := fmt.Sprintf("key-%d-%d", id%numWriters, j)
                
                _, err := db.Get(key)
                // Allow not found errors during concurrent writes
                if err != nil && !errors.Is(err, ErrNotFound) {
                    require.NoError(t, err)
                }
            }
        }(i)
    }
    
    wg.Wait()
}
```

## Performance Metrics

### Throughput Metrics

```go
type ThroughputMetrics struct {
    TransactionsPerSecond float64
    RequestsPerSecond     float64
    BlocksPerSecond       float64
    MessagesPerSecond     float64
}

func (m *ThroughputMetrics) Calculate(duration time.Duration, counts Counts) {
    seconds := duration.Seconds()
    m.TransactionsPerSecond = float64(counts.Transactions) / seconds
    m.RequestsPerSecond = float64(counts.Requests) / seconds
    m.BlocksPerSecond = float64(counts.Blocks) / seconds
    m.MessagesPerSecond = float64(counts.Messages) / seconds
}
```

### Latency Metrics

```go
type LatencyMetrics struct {
    Mean   time.Duration
    Median time.Duration
    P95    time.Duration
    P99    time.Duration
    Max    time.Duration
}

func (m *LatencyMetrics) Calculate(latencies []time.Duration) {
    sort.Slice(latencies, func(i, j int) bool {
        return latencies[i] < latencies[j]
    })
    
    n := len(latencies)
    m.Mean = calculateMean(latencies)
    m.Median = latencies[n/2]
    m.P95 = latencies[int(float64(n)*0.95)]
    m.P99 = latencies[int(float64(n)*0.99)]
    m.Max = latencies[n-1]
}
```

### Resource Metrics

```go
import (
    "runtime"
    "time"
)

type ResourceMetrics struct {
    CPUUsage    float64
    MemoryUsage uint64
    Goroutines  int
    GCPauses    time.Duration
}

func (m *ResourceMetrics) Collect() {
    var memStats runtime.MemStats
    runtime.ReadMemStats(&memStats)
    
    m.MemoryUsage = memStats.Alloc
    m.Goroutines = runtime.NumGoroutine()
    m.GCPauses = time.Duration(memStats.PauseTotalNs)
}
```

## Test Scenarios

### 1. API Performance Test

```go
func TestAPIPerformance(t *testing.T) {
    server := startTestServer(t)
    defer server.Close()
    
    client := NewAPIClient(server.URL)
    
    // Warmup
    for i := 0; i < 100; i++ {
        client.Query("acc://test.acme/account")
    }
    
    // Performance test
    const numRequests = 10000
    start := time.Now()
    
    var wg sync.WaitGroup
    for i := 0; i < numRequests; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            
            _, err := client.Query("acc://test.acme/account")
            require.NoError(t, err)
        }()
    }
    
    wg.Wait()
    duration := time.Since(start)
    
    rps := float64(numRequests) / duration.Seconds()
    t.Logf("API Performance: %.2f requests/second", rps)
    
    // Assert minimum performance
    assert.Greater(t, rps, 1000.0, "API performance below threshold")
}
```

### 2. Database Performance Test

```go
func TestDatabasePerformance(t *testing.T) {
    db := setupTestDatabase(t)
    defer db.Close()
    
    // Write performance
    const numWrites = 100000
    start := time.Now()
    
    for i := 0; i < numWrites; i++ {
        key := fmt.Sprintf("key-%d", i)
        value := []byte(fmt.Sprintf("value-%d", i))
        
        err := db.Put(key, value)
        require.NoError(t, err)
    }
    
    writeDuration := time.Since(start)
    writeRate := float64(numWrites) / writeDuration.Seconds()
    
    // Read performance
    start = time.Now()
    
    for i := 0; i < numWrites; i++ {
        key := fmt.Sprintf("key-%d", i)
        
        _, err := db.Get(key)
        require.NoError(t, err)
    }
    
    readDuration := time.Since(start)
    readRate := float64(numWrites) / readDuration.Seconds()
    
    t.Logf("Database Write Rate: %.2f ops/second", writeRate)
    t.Logf("Database Read Rate: %.2f ops/second", readRate)
    
    // Assert minimum performance
    assert.Greater(t, writeRate, 10000.0, "Write performance below threshold")
    assert.Greater(t, readRate, 50000.0, "Read performance below threshold")
}
```

### 3. Network Performance Test

```go
func TestNetworkPerformance(t *testing.T) {
    sim := simulator.New(t, 3).WithPartitions(2)
    sim.InitFromGenesis()
    
    alice := sim.Partition("BVN0").Account("alice")
    bob := sim.Partition("BVN1").Account("bob")
    
    sim.FundAccount(alice, 1000000)
    
    // Cross-partition transaction performance
    const numTransactions = 1000
    start := time.Now()
    
    for i := 0; i < numTransactions; i++ {
        txn := &SendTokens{
            From:   alice.URL(),
            To:     bob.URL(),
            Amount: 1,
        }
        
        result := sim.Submit(txn)
        require.NoError(t, result.Error)
    }
    
    // Execute all blocks
    sim.ExecuteBlocks(100)
    
    duration := time.Since(start)
    tps := float64(numTransactions) / duration.Seconds()
    
    t.Logf("Cross-partition TPS: %.2f transactions/second", tps)
    
    // Assert minimum performance
    assert.Greater(t, tps, 100.0, "Cross-partition performance below threshold")
}
```

## Analysis and Optimization

### Performance Analysis Workflow

1. **Baseline Measurement**: Establish current performance
2. **Bottleneck Identification**: Find performance constraints
3. **Optimization Implementation**: Apply performance improvements
4. **Validation**: Verify improvements
5. **Regression Testing**: Ensure no performance regressions

### Profiling Analysis

```bash
# CPU hotspots
go tool pprof cpu.prof
(pprof) top10
(pprof) list hotFunction

# Memory allocations
go tool pprof mem.prof
(pprof) top10 -alloc_space
(pprof) list allocatingFunction

# Goroutine analysis
go tool pprof goroutine.prof
(pprof) top10
(pprof) traces
```

### Performance Optimization Techniques

#### 1. Memory Optimization

```go
// Before: Inefficient allocation
func processData(items []Item) []Result {
    var results []Result
    for _, item := range items {
        result := processItem(item)
        results = append(results, result)
    }
    return results
}

// After: Pre-allocated slice
func processData(items []Item) []Result {
    results := make([]Result, 0, len(items))
    for _, item := range items {
        result := processItem(item)
        results = append(results, result)
    }
    return results
}
```

#### 2. Concurrency Optimization

```go
// Before: Sequential processing
func processItems(items []Item) []Result {
    var results []Result
    for _, item := range items {
        result := processItem(item)
        results = append(results, result)
    }
    return results
}

// After: Concurrent processing
func processItems(items []Item) []Result {
    const numWorkers = 10
    
    jobs := make(chan Item, len(items))
    results := make(chan Result, len(items))
    
    // Start workers
    for i := 0; i < numWorkers; i++ {
        go func() {
            for item := range jobs {
                result := processItem(item)
                results <- result
            }
        }()
    }
    
    // Send jobs
    for _, item := range items {
        jobs <- item
    }
    close(jobs)
    
    // Collect results
    var finalResults []Result
    for i := 0; i < len(items); i++ {
        result := <-results
        finalResults = append(finalResults, result)
    }
    
    return finalResults
}
```

#### 3. Caching Optimization

```go
type Cache struct {
    mu    sync.RWMutex
    items map[string]CacheItem
}

type CacheItem struct {
    Value  interface{}
    Expiry time.Time
}

func (c *Cache) Get(key string) (interface{}, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    item, exists := c.items[key]
    if !exists || time.Now().After(item.Expiry) {
        return nil, false
    }
    
    return item.Value, true
}

func (c *Cache) Set(key string, value interface{}, ttl time.Duration) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    c.items[key] = CacheItem{
        Value:  value,
        Expiry: time.Now().Add(ttl),
    }
}
```

## CI/CD Integration

### GitHub Actions Performance Testing

```yaml
name: Performance Tests
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  performance:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v3
    - uses: actions/setup-go@v3
      with:
        go-version: '1.21'
    
    - name: Run Benchmarks
      run: |
        go test -bench=. -benchmem ./... > benchmark.txt
        cat benchmark.txt
    
    - name: Load Test
      run: |
        # Start server in background
        go run ./cmd/accumulated &
        sleep 10
        
        # Run load test
        go run ./test/cmd/load/main.go \
          -server=http://localhost:8080 \
          -transactions=1000 \
          -duration=60 \
          -output=loadtest.json
    
    - name: Performance Analysis
      run: |
        # Analyze results
        go run ./tools/analyze-performance.go \
          -benchmark=benchmark.txt \
          -loadtest=loadtest.json
    
    - name: Upload Results
      uses: actions/upload-artifact@v3
      with:
        name: performance-results
        path: |
          benchmark.txt
          loadtest.json
```

### Performance Regression Detection

```bash
#!/bin/bash
# performance-check.sh

# Run current benchmarks
go test -bench=. -benchmem ./... > current.bench

# Compare with baseline
if [ -f baseline.bench ]; then
    benchstat baseline.bench current.bench > comparison.txt
    
    # Check for regressions
    if grep -q "~" comparison.txt; then
        echo "Performance regression detected!"
        cat comparison.txt
        exit 1
    fi
fi

# Update baseline
cp current.bench baseline.bench
```

## Best Practices

### 1. Test Environment

- **Consistent Hardware**: Use same hardware for comparisons
- **Isolated Environment**: Minimize external interference
- **Baseline Measurements**: Establish performance baselines
- **Multiple Runs**: Average results across multiple runs

### 2. Benchmark Design

```go
// Good: Focused benchmark
func BenchmarkSpecificFunction(b *testing.B) {
    // Setup once
    data := setupTestData()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        specificFunction(data)
    }
}

// Bad: Benchmark includes setup
func BenchmarkWithSetup(b *testing.B) {
    for i := 0; i < b.N; i++ {
        data := setupTestData() // Measured!
        specificFunction(data)
    }
}
```

### 3. Load Test Design

- **Gradual Ramp-up**: Increase load gradually
- **Realistic Scenarios**: Use production-like workloads
- **Resource Monitoring**: Monitor system resources
- **Error Handling**: Track and analyze errors

### 4. Performance Monitoring

```go
type PerformanceMonitor struct {
    metrics chan Metric
    done    chan struct{}
}

func (pm *PerformanceMonitor) Start() {
    go func() {
        ticker := time.NewTicker(time.Second)
        defer ticker.Stop()
        
        for {
            select {
            case <-ticker.C:
                pm.collectMetrics()
            case <-pm.done:
                return
            }
        }
    }()
}

func (pm *PerformanceMonitor) collectMetrics() {
    var m runtime.MemStats
    runtime.ReadMemStats(&m)
    
    metric := Metric{
        Timestamp:   time.Now(),
        MemoryUsage: m.Alloc,
        Goroutines:  runtime.NumGoroutine(),
    }
    
    select {
    case pm.metrics <- metric:
    default:
        // Channel full, skip metric
    }
}
```

### 5. Result Analysis

- **Statistical Significance**: Use multiple runs and statistical analysis
- **Trend Analysis**: Track performance over time
- **Bottleneck Identification**: Focus on the slowest components
- **Optimization Validation**: Verify improvements with measurements

---

## See Also

- [testing.md](testing.md) - Complete testing guide
- [unit-tests.md](unit-tests.md) - Unit testing guide
- [e2e-tests.md](e2e-tests.md) - End-to-end testing guide
- [debugging.md](debugging.md) - Test debugging techniques
- [test-content.md](test-content.md) - Complete test suite catalog

*This guide focuses on performance testing and optimization. For other testing approaches, see the related documentation.*
