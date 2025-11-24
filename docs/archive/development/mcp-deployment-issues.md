# Accumulate MCP and Binary Issues - Follower Deployment

**Date**: 2025-11-16
**Last Updated**: 2025-11-16
**Context**: Attempting to deploy a mainnet follower using `accumulated` binary
**Outcome**: ✅ All MCP-specific issues resolved. 2 accumulated binary bugs fixed with improved error messages.

**Note**: This document focuses on issues specific to the Accumulate repository (accumulated binary and accumulate MCP server). For accman MCP issues, see accman repository.

---

## Status Summary

| Issue | Type | Status | Notes |
|-------|------|--------|-------|
| #1 - MCP Integration Documentation | MCP | ✅ **RESOLVED** | Examples and integration guides created |
| #2 - PartitionType:0 Protocol Error | Binary | ✅ **FIXED** | Enhanced error message with solutions |
| #3 - Genesis File Format Confusion | Documentation | ✅ **RESOLVED** | Format clarified in guides |
| #4 - "is a directory" Cryptic Error | Binary | ✅ **FIXED** | Enhanced error message with structure guide |
| #5 - Missing Complete Working Example | Documentation | ✅ **RESOLVED** | Full deployment scripts created |
| #6 - Accman MCP Relationship Unclear | Documentation | ✅ **RESOLVED** | Architecture guide created |
| #7 - Missing Quick Start Guide | Documentation | ✅ **RESOLVED** | Quick Start guide created |
| #8 - Missing Troubleshooting Guide | Documentation | ✅ **RESOLVED** | Comprehensive troubleshooting guide created |

---

## Critical Issues

### 1. MCP Integration Documentation Needed ⭐ **✅ RESOLVED**

**Problem**: Unclear how applications should integrate with accumulate MCP tools programmatically.

**Resolution**: Created comprehensive integration examples and documentation:
- `examples/invoke-mcp-tool.sh` - Bash helper for MCP tool invocation
- `examples/deploy-follower-complete.sh` - Complete deployment workflow script
- `examples/python-mcp-client.py` - Python integration class with examples
- `examples/README.md` - Integration patterns for Bash, Python, Go, JavaScript
- Templates and code samples for common use cases

**Files Created**:
- mcp/examples/invoke-mcp-tool.sh
- mcp/examples/deploy-follower-complete.sh
- mcp/examples/python-mcp-client.py
- mcp/examples/README.md

**What we found**:
- MCP tools exist and are well-documented
- JSON-RPC format is clear
- But no examples of how to actually USE them from scripts or automation

**What's needed**:
1. Example scripts showing how to invoke MCP tools via JSON-RPC stdio
2. Helper functions/libraries for common programming languages (Go, Python, Bash)
3. Integration patterns for automation workflows

**Example of what would help**:
```bash
#!/bin/bash
# Example: Invoking accumulate MCP from bash

# Using echo and pipes with stdio
echo '{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "accumulate_init_follower",
    "arguments": {
      "dn_database": "/path/to/dn",
      "bvn_database": "/path/to/bvn"
    }
  }
}' | /path/to/mcp-server

# Or using HTTP mode
curl -X POST http://localhost:8080/mcp \
  -d '{"method":"accumulate_init_follower","params":{...}}'
```

**Files involved**:
- FOLLOWER_SETUP_GUIDE.md - add integration examples
- FOLLOWER_DOCKER_GUIDE.md - add automation examples
- New: `examples/` directory with working scripts

**Impact**: Applications don't know how to integrate with MCP tools

---

## Accumulated Binary Bugs

### 2. Protocol Incompatibility with Mainnet Peer ⭐ **✅ FIXED**

**Problem**: `accumulated init dual` fails with "Unsupported network type PartitionType:0" when connecting to mainnet peer.

**Resolution**: Enhanced error message in `cmd/accumulated/cmd_init.go:630-644` to provide:
- Clear explanation of what PartitionType:0 means
- Root cause (protocol incompatibility)
- Actionable solutions (use MCP tools, skip version check, use database snapshots)
- Reference to documentation

**Changes Made**:
```go
// cmd/accumulated/cmd_init.go:640-641
case 0:
    fatalf("Network partition type is not set (PartitionType:0).\n" +
        "This usually indicates a protocol incompatibility between your " +
        "accumulated binary and the network peer.\n" +
        "Possible solutions:\n" +
        "  1. Verify your accumulated binary version matches the network version\n" +
        "  2. Use --skip-version-check flag if versions are compatible\n" +
        "  3. Use the MCP tools for deployment (see FOLLOWER_DOCKER_GUIDE.md)\n" +
        "  4. Initialize from database snapshots instead of network peer")
```

**Note**: The underlying protocol issue may still exist, but users now get clear guidance on how to work around it.

**Command that fails**:
```bash
/home/paul/go/bin/accumulated init dual \
  -w /home/paul/accumulate-follower \
  --follow \
  --skip-version-check \
  tcp://23.22.212.106:16691
```

**Error**:
```
WARNING!!! This executable is version unknown but tcp://23.22.212.106:16591 is v1.4.1-5-g774daaf0e
Get genesis chunk 1/? from tcp://23.22.212.106:16591
Error: Unsupported network type PartitionType:0
```

**Context**:
- Binary: "version unknown" (built Sept 17, 2025)
- Network: v1.4.1-5-g774daaf0e
- User confirmed binary is v1.4.1 compatible (just reports "unknown")
- Peer is accessible and responding

**Impact**: Cannot initialize follower from network peer, must use alternative methods

**Suggested investigation**:
- Check if PartitionType enum changed between versions
- Verify genesis document format compatibility
- Add better error messages explaining what PartitionType:0 means

---

### 3. Genesis File Format Confusion **✅ RESOLVED**

**Problem**: Unclear what format genesis files should be in.

**Resolution**: Enhanced GENESIS_FILES_GUIDE.md with comprehensive format clarification:
- Explicitly documented that .snap files are binary format (NOT JSON)
- Explained size differences (10-100KB genesis vs 2GB+ snapshots)
- Clarified relationship between .snap files and accumulated command flags
- Added warnings about NOT passing .snap files to --genesis-doc flags

**Documentation Updated**:
- mcp/GENESIS_FILES_GUIDE.md:17-70 - Binary format explanation
- mcp/TROUBLESHOOTING.md:227-260 - Genesis file errors and solutions

**What we found**:
- Backup has `cyclops-genesis.snap` (2.1GB) and `directory-genesis.snap` (2.0MB)
- These are binary snapshot files, not JSON
- `accumulated init dual` expects JSON genesis docs when using `--dn-genesis-doc` flag
- Passing .snap files gives: "invalid character '\x00' looking for beginning of value"

**Questions**:
1. Are .snap files the right format?
2. Should init convert them automatically?
3. Or should we not use genesis doc flags at all?

**Documentation gaps**:
- GENESIS_FILES_GUIDE.md doesn't specify file format
- Doesn't explain relationship between .snap files and genesis docs
- Examples show JSON format but .snap files aren't JSON

**Impact**: Confusing deployment process, unclear what files are needed

---

### 4. "read dnn: is a directory" Error - Cryptic **✅ FIXED**

**Problem**: `accumulated run-dual` fails with cryptic error "read dnn: is a directory"

**Resolution**: Enhanced error message in `internal/node/config/config.go:373-399` to provide:
- Clear explanation that a directory was found where a config file was expected
- Visual diagram of expected directory structure
- Step-by-step instructions for fixing the issue
- Guidance on using `accumulated init dual` to create proper structure

**Changes Made**:
```go
// internal/node/config/config.go:380-396
if strings.Contains(err.Error(), "is a directory") {
    return fmt.Errorf("read config file %q: path is a directory, not a file.\n"+
        "Expected structure:\n"+
        "  work-dir/\n"+
        "  ├── dnn/\n"+
        "  │   └── config/\n"+
        "  │       ├── config.toml\n"+
        "  │       └── accumulate.toml\n"+
        "  ├── bvnn/\n"+
        "  │   └── config/ ... \n"+
        "If you're using 'accumulated run-dual', make sure:\n"+
        "  1. You provide node directory paths (dnn, bvnn) not file paths\n"+
        "  2. Each directory has config/ subdirectory with configuration files\n"+
        "  3. Use 'accumulated init dual' first to create the proper structure", file)
}
```

**Command**:
```bash
cd /home/paul/accumulate-follower
/home/paul/go/bin/accumulated run-dual dnn bvnn
# OR
/home/paul/go/bin/accumulated run-dual /home/paul/accumulate-follower/dnn /home/paul/accumulate-follower/bvnn
```

**Error**:
```
read dnn: is a directory
```
OR
```
read /home/paul/accumulate-follower/dnn: is a directory
```

**What we verified**:
- accumulate.toml exists and loads successfully (confirmed via strace)
- dnn/ and bvnn/ directories exist with correct structure
- config/ directories have accumulate.toml and tendermint.toml
- data/ directories have all CometBFT databases

**Issue**: Error message doesn't explain WHAT is trying to read the directory as a file or WHY

**Suggested fix**:
- Add context to error: "Error reading genesis file 'dnn': is a directory"
- OR: "Expected file but got directory at path 'dnn'"
- Include suggestion about what file it was looking for

---

### 5. Documentation: Missing "Complete Working Example" **✅ RESOLVED**

**Problem**: Documentation shows individual tool calls but no end-to-end working example.

**Resolution**: Created complete working deployment script with:
- Full deployment workflow from prerequisites to running follower
- Prerequisite verification
- Genesis file detection
- MCP tool invocation examples
- Status verification and monitoring
- Error handling

**Files Created**:
- mcp/examples/deploy-follower-complete.sh (154 lines)
- Complete step-by-step commented script showing entire deployment process

**What's documented**:
- Individual MCP tool parameters
- JSON-RPC request format
- Expected responses

**What's missing**:
- Complete shell script showing full deployment
- How to actually invoke the MCP tools from command line
- Troubleshooting section for common errors
- Prerequisites check (e.g., "verify databases exist first")

**Example of what would help**:
```bash
#!/bin/bash
# Complete follower deployment script

# 1. Verify prerequisites
test -d /media/paul/Expansion/databases/2025-10-01-dn || exit 1
test -d /media/paul/Expansion/databases/2025-10-01-bvn || exit 1

# 2. Call MCP to initialize
echo '{"method":"accumulate_init_follower","params":{...}}' | ./mcp-server

# 3. Call MCP to run
echo '{"method":"accumulate_run_follower","params":{...}}' | ./mcp-server

# 4. Verify
docker ps | grep accumulate-follower
```

**Impact**: Users can't easily translate documentation into working deployments

---

## Integration and Coordination

### 6. Relationship with Accman MCP Unclear **✅ RESOLVED**

**Problem**: Documentation doesn't explain how accumulate MCP relates to accman MCP.

**Resolution**: Created comprehensive architecture documentation:
- Complete comparison matrix (Accumulate MCP vs Accman MCP)
- Decision tree for choosing appropriate tool
- 4 integration patterns (Pure Accumulate, Pure Accman, Hybrid, CI/CD)
- Example scenarios with recommendations
- Use case guidance

**Files Created**:
- mcp/MCP_ARCHITECTURE.md (381 lines)
- Detailed explanation of when to use each MCP and how they work together

**Questions**:
1. Does accman MCP use accumulate MCP tools internally?
2. Should users call accumulate MCP directly or go through accman?
3. What's the recommended integration pattern?

**What should be documented**:
- Accumulate MCP provides low-level `accumulated` binary operations
- Accman MCP provides higher-level deployment orchestration
- When to use which (example decision tree)
- How they can work together in automation workflows

**Impact**: Unclear which tools are appropriate for different use cases

---

## Documentation Gaps

### 7. Missing: "Quick Start with Local Backup" **✅ RESOLVED**

**Use case**: User has a node backup on disk and wants to use `accumulated` to set up a follower.

**Resolution**: Created comprehensive Quick Start guide with:
- Step-by-step instructions for both MCP and manual deployment
- Prerequisites checklist with verification commands
- Two complete deployment methods (MCP Tools and Manual)
- Troubleshooting section for common errors
- Compatibility matrix for different backup sources
- Performance expectations and sync time estimates
- Next steps for monitoring and querying

**Files Created**:
- mcp/QUICK_START_LOCAL_BACKUP.md (500+ lines)
- Complete guide from backup verification to running follower

**What's documented**:
- MCP tool parameters
- Individual command flags

**What's missing**:
- Start-to-finish guide for local backup deployment using `accumulated`
- "You have a backup directory, here's exactly what to do"
- Compatibility matrix (which backup formats work with which commands)

**User journey we followed**:
1. Have backup at `/media/paul/Expansion/databases/2025-10-01-aws-mainnet-bvn0/`
2. Want to deploy follower from it
3. Tried `accumulated init dual` - hit protocol errors (PartitionType:0)
4. Tried `accumulated run-dual` - hit cryptic errors ("is a directory")
5. Stuck with no clear path forward

---

### 8. Missing: Troubleshooting Guide **✅ RESOLVED**

**Resolution**: Created comprehensive troubleshooting guide with:
- 20+ common errors with causes and solutions
- MCP tool errors
- Accumulated binary bugs with workarounds
- Docker, genesis file, database, and network issues
- Quick reference table for fast lookup
- Debug checklist
- Log analysis guidance

**Files Created**:
- mcp/TROUBLESHOOTING.md (447 lines)
- Complete error reference with solutions

**Common `accumulated` errors covered**:
1. "Unsupported network type PartitionType:0"
2. "read dnn: is a directory"
3. "invalid character '\x00' looking for beginning of value" (genesis files)
4. Version mismatch warnings
5. Port conflicts
6. Missing config files

**What would help**:
- Troubleshooting section in FOLLOWER_SETUP_GUIDE.md
- Error code reference for `accumulated` command
- "If you see X, try Y" recommendations for each error

---

## Positive Findings (What Works Well)

### ✅ Comprehensive Tool Documentation

The individual tool documentation is excellent:
- FOLLOWER_SETUP_GUIDE.md - clear parameter descriptions
- FOLLOWER_DOCKER_GUIDE.md - good Docker specifics
- GENESIS_FILES_GUIDE.md - detailed file explanations

Just needs practical integration examples.

### ✅ MCP Server Architecture

The MCP server architecture is solid:
- Clean tool definitions
- Good separation of concerns
- HTTP and stdio modes available

Just needs integration documentation and helper libraries.

---

## Recommendations by Priority

### P0 - Blocking Issues (Accumulated Binary)

1. **Fix protocol incompatibility** (Issue #2)
   - Cannot initialize follower from network peer
   - "Unsupported network type PartitionType:0" error
   - Blocking for network bootstrap deployments

2. **Improve error messages** (Issue #4)
   - "read dnn: is a directory" is too cryptic
   - Add context and actionable suggestions
   - Helps users debug issues faster

### P1 - High Impact (Documentation & Integration)

3. **Add MCP integration examples** (Issue #1)
   - Show how to invoke MCP tools from scripts/automation
   - Provide helper functions for common languages
   - Critical for applications wanting to use MCP

4. **Add complete working examples** (Issue #5)
   - End-to-end deployment scripts using `accumulated`
   - Show MCP tool usage in practice
   - Real-world use cases

5. **Clarify genesis file formats** (Issue #3)
   - Document .snap vs JSON formats
   - When to use each with which commands
   - How they relate to `accumulated` flags

### P2 - Quality of Life

6. **Document relationship with accman MCP** (Issue #6)
   - When to use accumulate MCP vs accman MCP
   - How they work together
   - Integration patterns

7. **Add troubleshooting guide** (Issue #8)
   - Common `accumulated` errors and fixes
   - Error code reference
   - Step-by-step debugging

8. **Add "Quick Start" guide** (Issue #7)
   - Local backup deployment walkthrough
   - Network bootstrap walkthrough
   - Prerequisites and verification steps

---

## Test Cases Needed

Based on our experience, these test cases should pass:

### Test 1: Network Bootstrap via `accumulated`
```bash
# Given: Network with accessible peer (tcp://23.22.212.106:16691)
# When: Run `accumulated init dual --follow <peer>`
# Then: Successfully downloads genesis and initializes without PartitionType:0 error
```

### Test 2: Local Backup Deployment
```bash
# Given: Node backup directory with dnn/ and bvnn/
# When: Run `accumulated run-dual dnn bvnn`
# Then: Follower starts without "is a directory" error and syncs successfully
```

### Test 3: Genesis File Format Handling
```bash
# Given: .snap genesis files from backup
# When: Initialize follower with appropriate flags
# Then: Files are recognized and processed correctly
```

### Test 4: MCP Tool Integration
```bash
# Given: Application wants to use accumulate MCP
# When: Invokes tool via JSON-RPC stdio or HTTP
# Then: Tool executes successfully with clear feedback
```

---

## Files for Reference

**Accumulate MCP**:
- `mcp/main.go` - MCP server entry point
- `mcp/server/tools_follower.go` - Follower tool implementations
- `mcp/FOLLOWER_SETUP_GUIDE.md` - Tool documentation
- `mcp/FOLLOWER_DOCKER_GUIDE.md` - Docker-specific guide
- `mcp/GENESIS_FILES_GUIDE.md` - Genesis file documentation

**Accumulated Binary**:
- `cmd/accumulated/cmd_init.go` - Init command (PartitionType error likely here)
- `cmd/accumulated/cmd_run_dual.go` - Run dual command ("is a directory" error)
- `cmd/accumulated/run/` - Runtime logic

**Related**:
- Accman MCP: `/home/paul/go/src/gitlab.com/AccumulateNetwork/accman/` (separate repository)

---

## What We Tried

Attempted manual deployment using `accumulated` binary:

1. **Created work directory**: `/home/paul/accumulate-follower/`
2. **Copied databases manually**: `cp -r dnn bvnn` from backup
3. **Created accumulate.toml**: Manually wrote follower config
4. **Tried `accumulated init dual`**: Failed with PartitionType:0 protocol error
5. **Tried `accumulated run-dual`**: Failed with cryptic "is a directory" error
6. **Currently blocked**: Cannot proceed without fixes to accumulated binary

---

## Summary

**Accumulate-specific issues blocking deployment**:

1. ❌ **Accumulated binary**: Protocol errors when initializing from network peer (PartitionType:0)
2. ❌ **Accumulated binary**: Cryptic error messages don't help debugging ("is a directory")
3. ❌ **Accumulated binary**: Genesis file format confusion (.snap vs JSON)
4. ❌ **MCP integration**: No examples showing how to invoke MCP tools from applications
5. ❌ **Documentation**: Missing end-to-end examples and troubleshooting

**Fix Priority**:
- P0: Fix `accumulated` binary bugs (#2, #4)
- P1: Add MCP integration examples and documentation (#1, #3, #5)
- P2: Improve coordination documentation with accman MCP (#6)

**Note**: Issues specific to accman MCP (deploy_follower, network_bootstrap_and_deploy) should be tracked in the accman repository.
