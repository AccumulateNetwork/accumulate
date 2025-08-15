# Analyze Tool Command Implementation Map
<!-- AI_TAG: command_mapping -->
<!-- AI_COMPLEXITY: medium -->
<!-- AI_AUDIENCE: developer -->
<!-- AI_PRIORITY: critical -->

> **Complete mapping of analyze tool commands to their source implementations**

## Quick Reference

| Command | Source File | Function | Purpose | Status |
|---------|-------------|----------|---------|--------|
| `analyze snap <file>` | `snap.go` | `cmdAnalyzeSnap` | Analyze snapshot file structure | ✅ Active |
| `analyze snap-version <file>` | `snap.go` | `cmdAnalyzeSnapVersion` | Display snapshot version info | ✅ Active |
| `analyze snap-report <file>` | `snap_report_cmd.go` | `cmdAnalyzeSnapReport` | Generate detailed snapshot reports | ✅ Active |
| `analyze sc <dest> <src...>` | `sc.go` | `sc_Cmd` | Process and combine snapshots | ✅ Active |
| `analyze extract <snapshot>` | `a_extract_cmd.go` | `cmdAnalyzeExtract` | Extract data from snapshots | ✅ Active |
| `analyze info <snapshot>` | `a_extract_info.go` | `InfoCommand` | Display snapshot information | ✅ Active |

## Command Details

### Snapshot Analysis Commands
<!-- AI_TAG: snapshot_commands -->

#### `analyze snap <file>`
- **Source**: `snap.go:54` (cmdAnalyzeSnap)
- **Purpose**: Analyze snapshot file structure and validate format
- **Usage**: `analyze snap /path/to/snapshot.bpt`
- **Output**: Snapshot structure analysis and validation results
- **Dependencies**: 
  - `snap.go` - Core snapshot processing
  - `utils.go` - Utility functions

**Example:**
```bash
analyze snap mainnet-snapshot-12345.bpt
# Output: Snapshot analysis with section breakdown
```

#### `analyze snap-version <file>`
- **Source**: `snap.go:65` (cmdAnalyzeSnapVersion)
- **Purpose**: Display snapshot version and format information
- **Usage**: `analyze snap-version /path/to/snapshot.bpt`
- **Output**: Version, format, and compatibility information
- **Dependencies**: Same as `analyze snap`

**Example:**
```bash
analyze snap-version mainnet-snapshot-12345.bpt
# Output: Version: 2, Format: BPT, Compatible: Yes
```

#### `analyze snap-report <file>`
- **Source**: `snap_report_cmd.go:91` (cmdAnalyzeSnapReport)
- **Purpose**: Generate comprehensive snapshot analysis reports
- **Usage**: `analyze snap-report [flags] /path/to/snapshot.bpt`
- **Flags**:
  - `--output-dir` - Directory for report output
  - `--format` - Report format (json, text, html)
- **Output**: Detailed analysis reports with statistics
- **Dependencies**:
  - `snap_report.go` - Report generation logic
  - `snap_processing.go` - Snapshot processing utilities

**Example:**
```bash
analyze snap-report --output-dir ./reports --format json mainnet-snapshot-12345.bpt
# Output: Comprehensive JSON report in ./reports/
```

### Snapshot Processing Commands
<!-- AI_TAG: processing_commands -->

#### `analyze sc <destination> <input1> [input2...]`
- **Source**: `sc.go:187` (sc_Cmd)
- **Purpose**: Process and combine multiple snapshot files
- **Usage**: `analyze sc output.bpt input1.bpt input2.bpt`
- **Features**:
  - Single input: Validates reconstruction matches original
  - Multiple inputs: Combines into single destination snapshot
  - Maintains separation of account and message records
- **Dependencies**:
  - `sc_parse.go` - Snapshot parsing logic
  - `sc_recon_*.go` - Reconstruction algorithms
  - `sc_snap*.go` - Snapshot manipulation

**Example:**
```bash
# Process single snapshot (validation)
analyze sc validated.bpt original.bpt

# Combine multiple snapshots
analyze sc combined.bpt snapshot1.bpt snapshot2.bpt snapshot3.bpt
```

### Data Extraction Commands
<!-- AI_TAG: extraction_commands -->

#### `analyze extract <snapshot>`
- **Source**: `a_extract_cmd.go:29` (cmdAnalyzeExtract)
- **Purpose**: Extract specific data from snapshot files
- **Usage**: `analyze extract [flags] /path/to/snapshot.bpt`
- **Flags**:
  - `--validator-keys` - Comma-separated validator public keys (hex)
  - `--partition-snapshots` - Directory for partition-specific snapshots
- **Output**: Extracted data files organized by type
- **Dependencies**:
  - `a_extract*.go` - Extraction logic for different data types
  - `a_extract_accounts.go` - Account data extraction
  - `a_extract_consensus.go` - Consensus data extraction
  - `a_extract_messages.go` - Message data extraction

**Example:**
```bash
analyze extract --validator-keys abc123,def456 --partition-snapshots ./partitions mainnet.bpt
# Output: Extracted data organized in ./partitions/
```

#### `analyze info <snapshot>`
- **Source**: `a_extract_info.go:16` (InfoCommand)
- **Purpose**: Display detailed information about snapshot files
- **Usage**: `analyze info /path/to/snapshot.bpt`
- **Output**: Snapshot metadata, consensus section data, statistics
- **Dependencies**:
  - `a_extract_info.go` - Information display logic
  - Core snapshot parsing functions

**Example:**
```bash
analyze info mainnet-snapshot-12345.bpt
# Output: Detailed snapshot information including consensus data
```

## Implementation Architecture
<!-- AI_TAG: architecture -->

### Core Components

#### Main Entry Point
- **File**: `main.go`
- **Function**: `main()`, `init()`
- **Purpose**: Command registration and CLI setup
- **Key Code**:
```go
var rootCmd = &cobra.Command{
    Use:   "analyze",
    Short: "Analysis utilities for Accumulate",
    Long:  "A collection of utilities for analyzing Accumulate databases, snapshots, and networks",
}
```

#### Command Registration Pattern
All commands follow this pattern in `main.go:init()`:
```go
func init() {
    rootCmd.AddCommand(cmdAnalyzeSnap)
    rootCmd.AddCommand(cmdAnalyzeSnapVersion)
    rootCmd.AddCommand(cmdAnalyzeSnapReport)
    rootCmd.AddCommand(sc_Cmd)
    rootCmd.AddCommand(cmdAnalyzeExtract)
    rootCmd.AddCommand(InfoCommand())
}
```

### File Organization

#### Snapshot Processing (`snap*.go`)
- `snap.go` - Basic snapshot analysis commands
- `snap_processing.go` - Core processing utilities
- `snap_report.go` - Report generation
- `snap_report_cmd.go` - Report command implementation

#### SC (Snapshot Combine) Processing (`sc*.go`)
- `sc.go` - Main SC command and coordination
- `sc_parse.go` - Snapshot parsing logic
- `sc_recon_*.go` - Reconstruction algorithms
- `sc_snap*.go` - Snapshot manipulation
- `sc_utils.go` - Utility functions

#### Data Extraction (`a_extract*.go`)
- `a_extract_cmd.go` - Extract command implementation
- `a_extract_info.go` - Info command implementation
- `a_extract_accounts.go` - Account data extraction
- `a_extract_consensus.go` - Consensus data extraction
- `a_extract_messages.go` - Message data extraction
- `a_extract_*.go` - Various specialized extractors

#### Utilities
- `utils.go` - Common utility functions
- `record_types.go` - Record type definitions
- `blockchaindb_adapter.go` - Database adapter
- `bloom.go` - Bloom filter implementation

## Development Guidelines
<!-- AI_TAG: development -->

### Adding New Commands

1. **Create Command File**: `new_command.go`
2. **Define Command Variable**:
```go
var cmdAnalyzeNewCommand = &cobra.Command{
    Use:   "new-command <args>",
    Short: "Brief description",
    Long:  "Detailed description",
    Args:  cobra.ExactArgs(1),
    RunE:  runNewCommand,
}
```

3. **Register in main.go**:
```go
func init() {
    rootCmd.AddCommand(cmdAnalyzeNewCommand)
}
```

4. **Update This Documentation**: Add entry to command mapping table

### Testing Commands

Each command should have corresponding test files:
- `command_test.go` - Unit tests for command logic
- `test/` directory - Integration test data

### Error Handling

Commands should follow consistent error handling:
```go
func runCommand(cmd *cobra.Command, args []string) error {
    if err := validateArgs(args); err != nil {
        return fmt.Errorf("invalid arguments: %w", err)
    }
    
    if err := processCommand(args); err != nil {
        return fmt.Errorf("command failed: %w", err)
    }
    
    return nil
}
```

## Troubleshooting
<!-- AI_TAG: troubleshooting -->

### Common Issues

#### Command Not Found
- **Symptom**: `unknown command "command-name"`
- **Cause**: Command not registered in `main.go:init()`
- **Solution**: Add `rootCmd.AddCommand(cmdAnalyzeCommandName)` to init function

#### Import Errors
- **Symptom**: `undefined: functionName`
- **Cause**: Missing import or function not exported
- **Solution**: Check imports and ensure functions are capitalized (exported)

#### File Not Found
- **Symptom**: `no such file or directory`
- **Cause**: Incorrect file path or missing file
- **Solution**: Verify file paths and ensure files exist

### Debug Mode

Enable debug output with:
```bash
analyze --debug command args
```

## Related Documentation

- [Analyze Tool Commands](./analyze-commands.md) - Complete tool usage guide
- [Technical Formats](../technical/) - File format specifications
- [Node Daemon Commands](./accumulated-daemon-commands.md) - Node operation commands
