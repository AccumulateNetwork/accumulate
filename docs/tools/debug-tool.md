# Debug Tool

## Overview

The `debug` tool is a comprehensive debugging utility for the Accumulate Network. It provides various debugging capabilities including network analysis, transaction debugging, database inspection, and system diagnostics.

## Installation

```bash
# Build the debug tool
go build -o bin/debug ./tools/cmd/debug

# Or build all tools
make tools
```

## Usage

```bash
./bin/debug [command] [flags]
```

## Commands

### Network Debugging

```bash
# Analyze network connectivity
./bin/debug network --analyze

# Check bootstrap servers
./bin/debug network --bootstrap acc://bootstrap.accumulate.io

# Test network endpoints
./bin/debug network --test-endpoints
```

### Transaction Debugging

```bash
# Debug specific transaction
./bin/debug transaction --txid <transaction-id>

# Analyze transaction flow
./bin/debug transaction --trace <transaction-id>

# Check transaction status
./bin/debug transaction --status <transaction-id>
```

### Database Debugging

```bash
# Inspect database state
./bin/debug database --path ./data --inspect

# Check database consistency
./bin/debug database --path ./data --check

# Analyze database performance
./bin/debug database --path ./data --analyze
```

### System Diagnostics

```bash
# System health check
./bin/debug system --health

# Performance analysis
./bin/debug system --performance

# Resource usage
./bin/debug system --resources
```

## Configuration

### Environment Variables

```bash
# Set log level
export ACC_LOG_LEVEL=debug

# Set bootstrap servers
export ACC_BOOTSTRAP=acc://bootstrap.accumulate.io

# Set cache directory
export ACC_CACHE_DIR=~/.accumulate/cache
```

### Configuration File

Create `~/.accumulate/debug.yaml`:

```yaml
bootstrap:
  - acc://bootstrap.accumulate.io
  - acc://bootstrap2.accumulate.io

logging:
  level: debug
  format: json

cache:
  directory: ~/.accumulate/cache
  ttl: 1h
```

## Common Use Cases

### Debugging Test Failures

```bash
# Debug E2E test failure
./bin/debug test --test-name TestFactomAddresses --verbose

# Analyze test environment
./bin/debug test --environment --check-setup

# Debug simulator issues
./bin/debug simulator --port 8080 --analyze
```

### Network Issues

```bash
# Check network connectivity
./bin/debug network --ping --all-nodes

# Analyze consensus issues
./bin/debug consensus --analyze --verbose

# Check validator status
./bin/debug validators --status --all
```

### Database Problems

```bash
# Check for corruption
./bin/debug database --path ./data --verify

# Analyze slow queries
./bin/debug database --path ./data --slow-queries

# Check index health
./bin/debug database --path ./data --index-health
```

## Output Formats

### JSON Output

```bash
./bin/debug network --analyze --output json
```

### Verbose Output

```bash
./bin/debug transaction --txid <id> --verbose
```

### Quiet Mode

```bash
./bin/debug system --health --quiet
```

## Integration with Testing

### VS Code Integration

Add to `.vscode/tasks.json`:

```json
{
  "label": "Debug Network",
  "type": "shell",
  "command": "./bin/debug",
  "args": ["network", "--analyze", "--verbose"],
  "group": "test",
  "presentation": {
    "echo": true,
    "reveal": "always",
    "focus": false,
    "panel": "shared"
  }
}
```

### CI/CD Integration

```yaml
debug_analysis:
  stage: debug
  script:
    - ./bin/debug system --health --output json > debug-report.json
  artifacts:
    reports:
      junit: debug-report.json
  only:
    - when: on_failure
```

## Troubleshooting

### Common Issues

| Issue | Solution |
|-------|----------|
| Permission denied | Run with appropriate permissions or use `sudo` |
| Network timeout | Check bootstrap servers and network connectivity |
| Database locked | Ensure no other processes are using the database |
| Cache corruption | Clear cache: `rm -rf ~/.accumulate/cache` |

### Debug Flags

```bash
# Enable debug logging
./bin/debug --debug [command]

# Increase verbosity
./bin/debug --verbose [command]

# Dry run mode
./bin/debug --pretend [command]
```

## Examples

### Complete Debugging Session

```bash
# 1. Check system health
./bin/debug system --health

# 2. Analyze network
./bin/debug network --analyze --verbose

# 3. Check database
./bin/debug database --path ./data --check

# 4. Debug specific transaction
./bin/debug transaction --txid abc123 --trace

# 5. Generate report
./bin/debug report --output debug-session.json
```

### Automated Debugging Script

```bash
#!/bin/bash
# debug-session.sh

echo "Starting debug session..."

# System check
./bin/debug system --health --quiet || exit 1

# Network analysis
./bin/debug network --analyze --output json > network-analysis.json

# Database check
./bin/debug database --path ./data --check --output json > db-check.json

echo "Debug session complete. Check output files."
```

## See Also

- [Debugging Guide](../../test/docs/debugging.md) - General debugging strategies
- [Network Tools](simulator.md) - Network simulation and testing
- [Database Tools](dbrepair.md) - Database management utilities
