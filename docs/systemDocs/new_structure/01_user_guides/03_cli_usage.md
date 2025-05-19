# Accumulate Healing Command-Line Usage Guide

<!-- AI-METADATA
type: guide
version: 1.0
topic: healing_cli
subtopics: ["synthetic_healing_cli", "anchor_healing_cli", "command_flags"]
related_code: ["tools/cmd/debug/heal_synth.go", "tools/cmd/debug/heal_anchor.go", "tools/cmd/debug/heal_common.go"]
critical_rules: ["never_fabricate_data", "follow_reference_implementation"]
tags: ["healing", "cli", "documentation", "guide", "ai_optimized"]
-->

## Overview

This guide provides comprehensive documentation for the command-line tools used to perform healing operations in Accumulate. These tools are implemented in the `tools/cmd/debug` directory and provide interfaces for synthetic transaction healing and anchor healing.

## Synthetic Transaction Healing

### Command Syntax

```bash
heal-synth [network] [partition-pair] [sequence] [flags]
```

### Arguments

- `network`: Name of the network to heal (required)
- `partition-pair`: Optional partition pair in format "source:destination"
- `sequence`: Optional starting sequence number

### Flags

- `--since=<duration>`: How far back in time to heal (default: 48 hours, 0 for forever)
- `--max-response-age=<duration>`: Set to 336 hours (2 weeks) to allow for longer periods of data retrieval
- `--wait`: Flag to wait for transactions (enabled by default for heal-synth)
- `--report-interval=<duration>`: Interval for detailed progress reports (default: 5 minutes)
- `--continuous`: Run healing in a loop every minute
- `--pretend` or `-n`: Do not submit envelopes, only scan
- `--light-db=<path>`: Light client database for persisting chain data (defaults to a path in the cache directory based on network name)
- `--debug`: Enable debug logging for network requests
- `--pprof=<address>`: Address to run net/http/pprof on (for performance profiling)

### Examples

```bash
# Heal synthetic transactions for the last 48 hours on mainnet
heal-synth mainnet

# Heal synthetic transactions for all time on testnet
heal-synth testnet --since=0

# Heal synthetic transactions for a specific partition pair
heal-synth devnet bvn1:directory

# Heal synthetic transactions starting from a specific sequence
heal-synth mainnet bvn2:directory 12345

# Heal with a longer response age window
heal-synth mainnet --max-response-age=336h
```

### Implementation Details

The synthetic transaction healing command is implemented in `tools/cmd/debug/heal_synth.go` and uses the core healing functionality from `internal/core/healing/synthetic.go`. The command follows these steps:

1. Parse command-line arguments and flags
2. Initialize the light client database and caching mechanisms
3. Set up the reporting infrastructure
4. Call the `HealSynthetic` function with the appropriate arguments
5. Display progress reports at the configured interval
6. Output a summary report when healing is complete

## Anchor Healing

### Command Syntax

```bash
heal-anchor [network] [flags]
```

### Arguments

- `network`: Name of the network to heal (required)

### Flags

- `--since=<duration>`: How far back in time to heal
- `--partition-pair=<source:destination>`: Specific partition pair to heal
- `--sequence=<number>`: Starting sequence number
- `--report-interval=<duration>`: Interval for detailed progress reports (default: 5 minutes)

### Examples

```bash
# Heal anchors for the last 48 hours on mainnet
heal-anchor mainnet

# Heal anchors for a specific partition pair
heal-anchor testnet --partition-pair=directory:bvn1

# Heal anchors starting from a specific sequence
heal-anchor devnet --sequence=5000

# Heal anchors with a custom report interval
heal-anchor mainnet --report-interval=10m
```

### Implementation Details

The anchor healing command is implemented in `tools/cmd/debug/heal_anchor.go` and uses the core healing functionality from `internal/core/healing/anchors.go`. The command follows these steps:

1. Parse command-line arguments and flags
2. Initialize the light client database and caching mechanisms
3. Set up the reporting infrastructure
4. Call the `HealAnchors` function with the appropriate arguments
5. Display progress reports at the configured interval
6. Output a summary report when healing is complete

## Common Patterns and Best Practices

### Usage Patterns

- For routine maintenance, run with default settings (48-hour window)
- For recovery after extended downtime, use `--since=0` to heal all history
- For targeted healing, specify partition pairs and sequence numbers
- For long-running operations, consider adjusting the report interval

### Critical Rules

- **Data Integrity**: Never fabricate data that isn't available from the network
- **Reference Implementation**: Follow the reference test code exactly
- **Error Handling**: Properly handle "key not found" errors and implement fallback mechanisms
- **Logging**: Add detailed logging for on-demand transaction fetching

### Performance Considerations

- The LRU cache is limited to 1000 entries, which may affect performance for large healing operations
- Consider using the `--max-response-age` flag to optimize data retrieval for long time periods
- For very large healing operations, consider breaking them into smaller time windows

## Troubleshooting

### Common Issues

- **Network Connectivity**: Ensure the node has connectivity to the specified network
- **API Version Compatibility**: Verify that the API versions are compatible
- **Memory Usage**: For large healing operations, monitor memory usage and consider adjusting cache sizes
- **Timeout Errors**: For slow networks, consider increasing timeout values

### Debug Options

- Enable verbose logging for more detailed information
- Use the `--report-interval` flag to get more frequent progress updates
- Check the error messages for specific API or network issues

## Related Documents

- [Main Index](./00_index.md)
- [API Documentation Index](./00a_api_index.md)
- [Healing Processes Index](./00b_healing_processes_index.md)
- [Technical Infrastructure Index](./00c_technical_index.md)
