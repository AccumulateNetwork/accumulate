# Analyze Tool Command Reference
<!-- AI_TAG: analyze_commands -->
<!-- AI_COMPLEXITY: medium -->
<!-- AI_AUDIENCE: developer -->
<!-- AI_PRIORITY: critical -->

> **Complete command reference for the Accumulate analyze tool**

## Quick Reference
<!-- AI_TAG: quick_reference -->

| Command | Purpose | Usage |
|---------|---------|-------|
| `analyze snap <file>` | Analyze snapshot structure | Basic snapshot validation |
| `analyze snap-version <file>` | Show snapshot version | Version compatibility check |
| `analyze snap-report <file>` | Generate detailed reports | Comprehensive analysis |
| `analyze sc <dest> <src...>` | Process/combine snapshots | Snapshot reconstruction |
| `analyze extract <snapshot>` | Extract specific data | Data mining and analysis |
| `analyze info <snapshot>` | Display snapshot info | Quick snapshot overview |

## Installation & Setup
<!-- AI_TAG: installation -->

### Building the Tool

```bash
cd /path/to/accumulate/tools/cmd/analyze
go build -o analyze .
```

### Global Installation

```bash
go install gitlab.com/accumulatenetwork/accumulate/tools/cmd/analyze@latest
```

### Verification

```bash
analyze --help
# Should display available commands
```

## Command Details
<!-- AI_TAG: command_details -->

### Snapshot Analysis Commands

#### `analyze snap <file>`
<!-- AI_TAG: snap_command -->

**Purpose**: Analyze snapshot file structure and validate format integrity.

**Usage**:
```bash
analyze snap <snapshot-file>
```

**Examples**:
```bash
# Analyze MainNet snapshot
analyze snap mainnet-snapshot-12345.bpt

# Analyze local snapshot
analyze snap ./data/local-snapshot.bpt
```

**Output**:
- Snapshot file validation status
- Section breakdown and statistics
- Format compatibility information
- Error detection and reporting

**Common Use Cases**:
- Validate snapshot integrity before processing
- Debug snapshot format issues
- Verify snapshot compatibility

---

#### `analyze snap-version <file>`
<!-- AI_TAG: snap_version_command -->

**Purpose**: Display snapshot version and format compatibility information.

**Usage**:
```bash
analyze snap-version <snapshot-file>
```

**Examples**:
```bash
# Check snapshot version
analyze snap-version mainnet-snapshot-12345.bpt
```

**Output**:
```
Version: 2
Format: BPT
Compatible: Yes
Created: 2024-01-15T10:30:00Z
Size: 1.2GB
```

**Common Use Cases**:
- Verify snapshot version before processing
- Check compatibility with current tools
- Quick snapshot metadata overview

---

#### `analyze snap-report <file>`
<!-- AI_TAG: snap_report_command -->

**Purpose**: Generate comprehensive analysis reports with detailed statistics and insights.

**Usage**:
```bash
analyze snap-report [flags] <snapshot-file>
```

**Flags**:
- `--output-dir <dir>` - Directory for report output (default: current directory)
- `--format <format>` - Report format: json, text, html (default: text)
- `--include-accounts` - Include detailed account analysis
- `--include-transactions` - Include transaction statistics
- `--verbose` - Enable verbose output

**Examples**:
```bash
# Generate basic text report
analyze snap-report mainnet-snapshot-12345.bpt

# Generate JSON report in specific directory
analyze snap-report --output-dir ./reports --format json mainnet-snapshot-12345.bpt

# Generate comprehensive HTML report
analyze snap-report --format html --include-accounts --include-transactions mainnet.bpt
```

**Output Files**:
- `snapshot-report.txt/json/html` - Main report file
- `account-summary.csv` - Account statistics (if --include-accounts)
- `transaction-summary.csv` - Transaction statistics (if --include-transactions)

**Report Contents**:
- Snapshot metadata and version info
- Section-by-section analysis
- Account and transaction statistics
- Network state summary
- Validation results and warnings

---

### Snapshot Processing Commands

#### `analyze sc <destination> <input1> [input2...]`
<!-- AI_TAG: sc_command -->

**Purpose**: Process and combine multiple snapshot files with validation and reconstruction.

**Usage**:
```bash
analyze sc <destination-file> <input-file1> [input-file2] [input-file3] ...
```

**Behavior**:
- **Single Input**: Validates that reconstructed snapshot matches original byte-for-byte
- **Multiple Inputs**: Combines snapshots into single destination file
- **Type 7 Sections**: Maintains separation of account and message records

**Examples**:
```bash
# Validate single snapshot (reconstruction test)
analyze sc validated-output.bpt original-snapshot.bpt

# Combine multiple partition snapshots
analyze sc combined-mainnet.bpt bvn0-snapshot.bpt bvn1-snapshot.bpt dn-snapshot.bpt

# Process with validation
analyze sc processed.bpt input.bpt
```

**Output**:
- Progress indicators during processing
- Validation results for single input mode
- Combination statistics for multiple input mode
- Error reporting for any issues encountered

**Common Use Cases**:
- Validate snapshot integrity through reconstruction
- Combine partition snapshots into unified snapshot
- Process snapshots for analysis or deployment

---

### Data Extraction Commands

#### `analyze extract <snapshot>`
<!-- AI_TAG: extract_command -->

**Purpose**: Extract specific data types from snapshot files for analysis and processing.

**Usage**:
```bash
analyze extract [flags] <snapshot-file>
```

**Flags**:
- `--validator-keys <keys>` - Comma-separated validator public keys (hex encoded)
- `--partition-snapshots <dir>` - Directory to write partition-specific snapshots (default: /tmp/partition-snapshots)
- `--output-format <format>` - Output format: json, csv, binary (default: json)
- `--extract-accounts` - Extract account data
- `--extract-transactions` - Extract transaction data
- `--extract-consensus` - Extract consensus data

**Examples**:
```bash
# Extract with specific validator keys
analyze extract --validator-keys abc123def456,789ghi012jkl --partition-snapshots ./extracted mainnet.bpt

# Extract all data types
analyze extract --extract-accounts --extract-transactions --extract-consensus snapshot.bpt

# Extract to specific directory with CSV format
analyze extract --output-format csv --partition-snapshots ./data/extracted snapshot.bpt
```

**Output Structure**:
```
partition-snapshots/
├── accounts/
│   ├── bvn0-accounts.json
│   ├── bvn1-accounts.json
│   └── dn-accounts.json
├── transactions/
│   ├── bvn0-transactions.json
│   └── bvn1-transactions.json
├── consensus/
│   ├── validator-data.json
│   └── consensus-state.json
└── metadata/
    ├── extraction-summary.json
    └── partition-info.json
```

**Extracted Data Types**:
- **Accounts**: Account states, balances, and metadata
- **Transactions**: Transaction history and status
- **Consensus**: Validator information and consensus state
- **Messages**: Inter-partition messages and routing
- **Network State**: Overall network configuration

---

#### `analyze info <snapshot>`
<!-- AI_TAG: info_command -->

**Purpose**: Display detailed information about snapshot files including consensus data and statistics.

**Usage**:
```bash
analyze info <snapshot-file>
```

**Examples**:
```bash
# Display snapshot information
analyze info mainnet-snapshot-12345.bpt

# Pipe output for processing
analyze info snapshot.bpt | grep "Consensus"
```

**Output**:
```
Snapshot Information:
  File: mainnet-snapshot-12345.bpt
  Size: 1.2GB
  Version: 2
  Format: BPT
  Created: 2024-01-15T10:30:00Z

Consensus Section:
  Validators: 25
  Block Height: 1,234,567
  Block Hash: 0xabc123...
  State Root: 0xdef456...

Network State:
  Total Accounts: 45,678
  Total Transactions: 123,456,789
  Active Partitions: 3 (DN, BVN0, BVN1)

Validation:
  Checksum: Valid
  Structure: Valid
  Compatibility: v2.0+
```

**Information Categories**:
- **File Metadata**: Size, version, creation time
- **Consensus Data**: Validator info, block height, state hashes
- **Network Statistics**: Account counts, transaction volumes
- **Validation Results**: Integrity checks and compatibility

---

## Global Flags
<!-- AI_TAG: global_flags -->

### Common Flags

- `--bootstrap <servers>` - Set bootstrap servers for network operations
- `--help` - Display help information for any command
- `--version` - Display tool version information

### Debug and Logging

- `--debug` - Enable debug output
- `--verbose` - Enable verbose logging
- `--quiet` - Suppress non-essential output
- `--log-level <level>` - Set logging level (debug, info, warn, error)

### Examples

```bash
# Enable debug mode
analyze --debug snap snapshot.bpt

# Use custom bootstrap servers
analyze --bootstrap tcp://custom-bootstrap:16591 snap-report snapshot.bpt

# Quiet mode for scripting
analyze --quiet info snapshot.bpt
```

## Error Handling & Troubleshooting
<!-- AI_TAG: troubleshooting -->

### Common Errors

#### File Not Found
```
Error: open snapshot.bpt: no such file or directory
```
**Solution**: Verify file path and ensure file exists

#### Invalid Snapshot Format
```
Error: invalid snapshot format: unsupported version
```
**Solution**: Check snapshot version with `analyze snap-version` and update tools if needed

#### Permission Denied
```
Error: permission denied: cannot write to output directory
```
**Solution**: Check directory permissions or use `--output-dir` with writable location

#### Memory Issues
```
Error: cannot allocate memory
```
**Solution**: Process smaller snapshots or increase available memory

### Debug Mode

Enable detailed error information:
```bash
analyze --debug --verbose command args
```

### Log Analysis

Check logs for detailed error information:
```bash
analyze --log-level debug command args 2>&1 | tee analyze.log
```

## Performance Considerations
<!-- AI_TAG: performance -->

### Memory Usage

- **Large Snapshots**: May require significant RAM (2-4GB for MainNet snapshots)
- **Extraction Operations**: Memory usage scales with extracted data size
- **Report Generation**: HTML reports require more memory than text/JSON

### Processing Time

- **Snapshot Analysis**: 30 seconds - 5 minutes depending on size
- **Report Generation**: 1-10 minutes for comprehensive reports
- **Data Extraction**: 5-30 minutes depending on data types and filters
- **SC Processing**: 10-60 minutes for large snapshot combinations

### Optimization Tips

1. **Use Specific Flags**: Only extract needed data types
2. **Output Directory**: Use fast storage (SSD) for output
3. **Memory**: Ensure sufficient RAM for large snapshots
4. **Parallel Processing**: Some operations can be parallelized

## Integration Examples
<!-- AI_TAG: integration -->

### Bash Scripting

```bash
#!/bin/bash
# Automated snapshot analysis pipeline

SNAPSHOT_FILE="$1"
OUTPUT_DIR="./analysis-$(date +%Y%m%d)"

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Basic validation
echo "Validating snapshot..."
if ! analyze snap "$SNAPSHOT_FILE"; then
    echo "Snapshot validation failed"
    exit 1
fi

# Generate reports
echo "Generating reports..."
analyze snap-report --output-dir "$OUTPUT_DIR" --format json "$SNAPSHOT_FILE"

# Extract data
echo "Extracting data..."
analyze extract --partition-snapshots "$OUTPUT_DIR/extracted" "$SNAPSHOT_FILE"

echo "Analysis complete. Results in $OUTPUT_DIR"
```

### Python Integration

```python
import subprocess
import json
import os

def analyze_snapshot(snapshot_path, output_dir):
    """Analyze snapshot and return results"""
    
    # Validate snapshot
    result = subprocess.run(['analyze', 'snap', snapshot_path], 
                          capture_output=True, text=True)
    if result.returncode != 0:
        raise Exception(f"Snapshot validation failed: {result.stderr}")
    
    # Generate JSON report
    report_path = os.path.join(output_dir, 'report.json')
    subprocess.run(['analyze', 'snap-report', '--format', 'json', 
                   '--output-dir', output_dir, snapshot_path])
    
    # Load and return report data
    with open(report_path, 'r') as f:
        return json.load(f)

# Usage
results = analyze_snapshot('mainnet.bpt', './analysis')
print(f"Analyzed snapshot with {results['total_accounts']} accounts")
```

## Related Documentation

- [Command Implementation Map](./command-implementation-map.md) - Source code mapping
- [Technical Formats](../technical/) - File format specifications
- [Node Daemon Commands](./accumulated-daemon-commands.md) - Node operation commands
- [Deployment Guide](../deployment/cyclops-deployment-guide.md) - Network deployment
