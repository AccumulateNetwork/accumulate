# Performance Monitoring Scripts

This directory contains automation scripts for the Accumulate performance monitoring and tuning system.

## Scripts Overview

### perfmon-compare.sh

Compare multiple performance test reports and generate comparative analysis.

**Purpose:**
- Compare results across multiple test runs
- Track performance trends over time
- Identify improvements and regressions
- Generate comparison reports in multiple formats

**Usage:**
```bash
./perfmon-compare.sh [OPTIONS] REPORT1 REPORT2 [REPORT3 ...]
```

**Options:**
- `-o, --output DIR`: Output directory for comparison results (default: `./comparison-results`)
- `-f, --format FORMAT`: Output format: `table`, `json`, `csv` (default: `table`)
- `-b, --baseline FILE`: Baseline report for comparison
- `-s, --sort FIELD`: Sort by field: `tps`, `latency`, `errors` (default: `tps`)
- `-h, --help`: Show help message

**Examples:**

Compare two reports:
```bash
./perfmon-compare.sh report1.json report2.json
```

Compare multiple iterations with baseline:
```bash
./perfmon-compare.sh --baseline baseline.json \
  iteration1.json iteration2.json iteration3.json
```

Generate JSON output:
```bash
./perfmon-compare.sh --format json \
  --output ./comparison-results \
  baseline.json current.json
```

**Output:**

The script generates:
- Console table with side-by-side comparison
- Percentage changes (color-coded: green for improvements, red for regressions)
- Statistical summary (min, max, avg)
- Optional JSON/CSV files for further analysis

Example output:
```
================================================================================
PERFORMANCE COMPARISON REPORT
================================================================================

Report                         Target    Achieved    Success%     P99(ms)  Blocks/m
--------------------------------------------------------------------------------
baseline.json                     100       85.30       99.51      520.80     25.50
iteration1.json                   100       92.10       99.65      480.20     28.30
iteration2.json                   100       97.80       99.72      425.60     30.10

BASELINE COMPARISON
--------------------------------------------------------------------------------
Report                          TPS Change  Latency Chg  Success Chg
--------------------------------------------------------------------------------
iteration1.json                     +8.00%       -7.81%       +0.14%
iteration2.json                    +14.66%      -18.31%       +0.21%

STATISTICAL SUMMARY
--------------------------------------------------------------------------------
TPS:     Min: 85.30 | Max: 97.80 | Avg: 91.73
P99 Latency: Min: 425.60 ms | Max: 520.80 ms | Avg: 475.53 ms

================================================================================
```

### perfmon-workflow.sh

Automated iterative performance tuning workflow.

**Purpose:**
- Automate the complete tuning lifecycle
- Run baseline tests, analyze results, apply tuning, validate improvements
- Track progress across multiple iterations
- Generate comprehensive workflow summary

**Usage:**
```bash
./perfmon-workflow.sh [OPTIONS] SERVER TPS DURATION [ITERATIONS]
```

**Options:**
- `-w, --workflow-dir DIR`: Working directory for all outputs (default: `./perfmon-workflow`)
- `-t, --threshold FLOAT`: TPS achievement threshold 0.0-1.0 (default: `0.85`)
- `-a, --auto-apply`: Automatically apply tuning recommendations (USE WITH CAUTION)
- `-d, --delay SECONDS`: Wait time between iterations (default: `30`)
- `-c, --cmd COMMAND`: Debug command to use (default: `debug`)
- `-h, --help`: Show help message

**Arguments:**
- `SERVER`: Server endpoint (e.g., `localhost:16695`)
- `TPS`: Target transactions per second
- `DURATION`: Test duration (e.g., `5m`, `10m`, `1h`)
- `ITERATIONS`: Number of tuning iterations (default: `3`)

**Examples:**

Basic 3-iteration workflow:
```bash
./perfmon-workflow.sh localhost:16695 100 5m
```

Extended workflow with higher threshold:
```bash
./perfmon-workflow.sh --threshold 0.95 \
  --workflow-dir ./tuning-results \
  localhost:16695 500 10m 5
```

Custom debug command:
```bash
./perfmon-workflow.sh --cmd ./debug \
  localhost:16695 100 5m
```

**Workflow Steps:**

1. **Baseline Measurement**
   - Runs initial performance test
   - Analyzes baseline results
   - Generates initial recommendations

2. **Iterative Tuning** (repeated for each iteration)
   - Displays recommendations
   - Prompts for manual tuning application
   - Waits for system stabilization
   - Runs performance test
   - Analyzes results vs. baseline
   - Generates new recommendations
   - Checks if target threshold is met

3. **Final Summary**
   - Generates workflow summary JSON
   - Displays comparison table
   - Reports best iteration
   - Provides next steps

**Output Structure:**
```
perfmon-workflow/
├── baseline/
│   ├── report_20260322_143000.json
│   ├── metrics_20260322_143000.csv
│   └── recommendations.json
├── iteration-1/
│   ├── report_20260322_144500.json
│   ├── metrics_20260322_144500.csv
│   ├── analysis.json
│   └── recommendations.json
├── iteration-2/
│   └── ...
├── iteration-3/
│   └── ...
└── summary.json
```

**Summary Output:**
```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
WORKFLOW SUMMARY
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Configuration:
  Server: localhost:16695
  Target TPS: 100
  Duration: 5m
  Threshold: 0.85

Results by Iteration:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Iteration       Achieved TPS    Target %   Success %  P99 Lat (ms)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Baseline               85.30       85.30%       99.51%        520.80
Iteration 1 ✓          92.10       92.10%       99.65%        480.20
Iteration 2 ✓          97.80       97.80%       99.72%        425.60
Iteration 3 ✓         102.50      102.50%       99.78%        395.30
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Legend: ✓ = Improvement, ✗ = Regression, ~ = Neutral
```

## Common Use Cases

### 1. Quick Comparison of Two Runs

```bash
# Run two tests
debug perfmon localhost:16695 100 5m --output ./test1
debug perfmon localhost:16695 100 5m --output ./test2

# Compare
./perfmon-compare.sh ./test1/report_*.json ./test2/report_*.json
```

### 2. Automated Multi-Iteration Tuning

```bash
# Run automated workflow
./perfmon-workflow.sh localhost:16695 100 5m 5

# Review results
cat ./perfmon-workflow/summary.json | jq
```

### 3. Track Long-Term Performance Trends

```bash
# Run daily performance tests
for day in {1..7}; do
  debug perfmon localhost:16695 100 10m --output ./daily-test-day-$day
  sleep 86400  # 24 hours
done

# Compare all results
./perfmon-compare.sh --format csv --output ./weekly-trends \
  ./daily-test-day-*/report_*.json
```

### 4. A/B Testing Configuration Changes

```bash
# Test configuration A
debug perfmon localhost:16695 100 5m --output ./config-a

# Apply configuration B manually
# ...

# Test configuration B
debug perfmon localhost:16695 100 5m --output ./config-b

# Compare
./perfmon-compare.sh --baseline ./config-a/report_*.json \
  ./config-b/report_*.json
```

### 5. Validate Tuning Before Production

```bash
# Full workflow validation
./perfmon-workflow.sh --threshold 0.95 \
  --workflow-dir ./production-validation \
  production-node:16695 1000 1h 3

# Review all iterations
./perfmon-compare.sh --format json \
  --output ./final-comparison \
  ./production-validation/*/report_*.json
```

## Script Requirements

### Dependencies

Both scripts require:
- `bash` (version 4.0+)
- `jq` - JSON processor
- `bc` - Basic calculator for floating-point math
- `debug` command (Accumulate debug tool)

Install dependencies:
```bash
# Ubuntu/Debian
sudo apt-get install jq bc

# macOS
brew install jq bc

# Fedora/RHEL
sudo dnf install jq bc
```

### Permissions

Make scripts executable:
```bash
chmod +x perfmon-compare.sh
chmod +x perfmon-workflow.sh
```

## Integration with CI/CD

### GitLab CI Example

```yaml
performance-test:
  stage: test
  script:
    - ./scripts/perfmon-workflow.sh $TEST_NODE 100 5m 2
    - ./scripts/perfmon-compare.sh --format json
        ./perfmon-workflow/baseline/report_*.json
        ./perfmon-workflow/iteration-*/report_*.json
  artifacts:
    paths:
      - perfmon-workflow/
      - comparison-results/
    reports:
      junit: perfmon-workflow/summary.json
  only:
    - merge_requests
    - main
```

### GitHub Actions Example

```yaml
name: Performance Testing

on:
  pull_request:
  schedule:
    - cron: '0 2 * * *'  # Daily at 2 AM

jobs:
  performance:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2

      - name: Install dependencies
        run: |
          sudo apt-get update
          sudo apt-get install -y jq bc

      - name: Run performance workflow
        run: |
          ./scripts/perfmon-workflow.sh localhost:16695 100 5m 3

      - name: Compare results
        run: |
          ./scripts/perfmon-compare.sh --format json
            ./perfmon-workflow/*/report_*.json

      - name: Upload artifacts
        uses: actions/upload-artifact@v2
        with:
          name: performance-results
          path: |
            perfmon-workflow/
            comparison-results/
```

## Troubleshooting

### Script Fails with "command not found"

**Problem:** `jq` or `bc` not installed

**Solution:**
```bash
# Check if installed
which jq bc

# Install if missing (Ubuntu/Debian)
sudo apt-get install jq bc
```

### Permission Denied Errors

**Problem:** Scripts not executable

**Solution:**
```bash
chmod +x perfmon-compare.sh perfmon-workflow.sh
```

### JSON Parsing Errors

**Problem:** Invalid or corrupted report files

**Solution:**
```bash
# Validate JSON
jq empty report.json

# Check file exists and is readable
ls -l report.json
cat report.json | head
```

### Workflow Hangs at Tuning Prompt

**Problem:** Waiting for manual input in automated environment

**Solution:**
Either:
1. Run interactively and apply tuning manually
2. Use `--auto-apply` flag (only if you have automated config application)
3. Modify script to skip prompts for CI/CD

### Comparison Shows "N/A" for All Metrics

**Problem:** Report files have zero or null values

**Solution:**
- Ensure performance tests completed successfully
- Check that tests ran for sufficient duration
- Verify network connectivity to test server

## Best Practices

1. **Version Control**
   - Commit baseline reports to git
   - Track configuration changes alongside performance data
   - Use branches for tuning experiments

2. **Naming Conventions**
   - Use descriptive names: `baseline-v1.0.json`, `tuned-consensus-timeout.json`
   - Include timestamps: `report_20260322_143000.json`
   - Tag significant milestones: `production-ready-v1.json`

3. **Documentation**
   - Document what changed between iterations
   - Note configuration parameters in commit messages
   - Keep a tuning journal/changelog

4. **Automation Safety**
   - Always review recommendations before applying
   - Test in non-production first
   - Use `--auto-apply` only with comprehensive testing
   - Implement rollback procedures

5. **Data Retention**
   - Archive old results but keep key milestones
   - Compress large result sets
   - Maintain baseline references indefinitely

## Additional Resources

- [Performance Monitoring Documentation](../docs/perfmon.md)
- [Accumulate Configuration Guide](https://docs.accumulatenetwork.io/config)
- [Tuning Best Practices](https://docs.accumulatenetwork.io/performance)

## Contributing

To improve these scripts:

1. Test changes in non-production environment
2. Maintain backward compatibility
3. Update this README with new features
4. Add examples for new use cases
5. Follow existing code style

## License

Copyright 2025 The Accumulate Authors

Use of this source code is governed by an MIT-style license that can be found in the LICENSE file or at https://opensource.org/licenses/MIT.
