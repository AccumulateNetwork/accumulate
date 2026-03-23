# Performance Monitoring and Tuning Tool

This tool analyzes performance metrics, detects bottlenecks, and provides tuning recommendations.

## Usage

### Collect Live Metrics

```bash
go run ./test/cmd/perfmon --duration=30s --interval=1s
```

### Analyze Existing Metrics

```bash
go run ./test/cmd/perfmon --analyze --metrics=metrics.json
```

### Compare Against Baseline

```bash
go run ./test/cmd/perfmon --analyze --metrics=current.json --baseline=baseline.json
```

### Generate Sample Data

```bash
go run ./test/cmd/perfmon --sample --format=json --output=sample.json
```

## Tuning Algorithm

### Bottleneck Detection

The tool analyzes metrics using the following thresholds:

1. **Memory Usage** (averaged across all samples)
   - Threshold: 1 GB
   - Severity levels:
     - Medium: > 1 GB
     - High: > 2 GB
     - Critical: > 4 GB

2. **Goroutine Count** (averaged across all samples)
   - Threshold: 1000 goroutines
   - Severity levels:
     - Medium: > 1000
     - High: > 5000
     - Critical: > 10000

3. **GC Pause Times** (maximum pause across all samples)
   - Threshold: 10 ms
   - Severity levels:
     - Low: > 10 ms
     - Medium: > 50 ms
     - High: > 100 ms

4. **Allocation Rate** (averaged across all samples)
   - Threshold: 100 MB/s
   - Severity levels:
     - Low: > 100 MB/s
     - Medium: > 500 MB/s
     - High: > 1 GB/s

### Regression Detection

Compares current metrics against a baseline:
- Threshold: 10% degradation
- Metrics compared:
  - Memory usage
  - Goroutine count
  - Allocation rate

### Recommendation Generation

Based on detected bottlenecks, the tool provides:

1. **Memory Optimization**
   - Use object pooling (sync.Pool)
   - Reduce allocations
   - Implement memory limits
   - Expected: 30-50% reduction

2. **Concurrency Management**
   - Investigate goroutine leaks
   - Ensure proper context cancellation
   - Use bounded worker pools
   - Expected: Stable goroutine count

3. **GC Tuning**
   - Reduce allocation rate
   - Increase GOGC value
   - Use memory ballast
   - Expected: 50-70% reduction in GC pauses

4. **Allocation Optimization**
   - Reuse buffers with sync.Pool
   - Preallocate slices
   - Avoid string concatenation in loops
   - Expected: 40-60% reduction in allocation rate

## Test Data Examples

### Sample Metrics Structure

```json
{
  "timestamp": "2025-03-22T14:30:00Z",
  "cpu_usage": 45.5,
  "memory_usage": 536870912,
  "goroutine_count": 150,
  "alloc_rate": 52428800,
  "gc_pauses": [5000000, 3000000]
}
```

### Sample Report Structure

```json
{
  "metrics": [...],
  "bottlenecks": [
    {
      "type": "memory",
      "severity": "high",
      "description": "High memory usage detected",
      "value": 2147483648,
      "threshold": 1073741824
    }
  ],
  "regressions": [
    {
      "metric": "memory_usage",
      "baseline": 1000000000,
      "current": 1500000000,
      "degradation": 50.0
    }
  ],
  "tuning": [
    {
      "category": "Memory Optimization",
      "priority": "high",
      "description": "...",
      "expected_improvement": "30-50% reduction in memory usage"
    }
  ],
  "generated_at": "2025-03-22T14:31:00Z"
}
```

## Integration with Debug Tools

The perfmon functionality is also integrated into the debug tool:

```bash
# Via debug command
go run ./tools/cmd/debug perfmon --duration=30s

# Generate tuning recommendations
go run ./tools/cmd/debug perftune metrics.json --baseline=baseline.json --verbose
```

## Testing

Run tests with coverage:

```bash
go test ./test/cmd/perfmon -v -cover
```

Expected coverage: >70%
