#!/bin/bash

# Script to reorganize the docs directory into a clean topic-based structure

echo "=== Documentation Reorganization Script ==="
echo "This will reorganize docs/ into topic-based folders"
echo ""

# Create new directory structure
echo "Creating directory structure..."
mkdir -p architecture
mkdir -p crosschain
mkdir -p tools
mkdir -p debugging  
mkdir -p setup
mkdir -p meta
mkdir -p _archive

# Move architecture files
echo "Moving architecture documentation..."
mv -f network-sync.md architecture/ 2>/dev/null
mv -f P2P-ARCHITECTURE.md architecture/ 2>/dev/null
mv -f light-client-design.md architecture/ 2>/dev/null
mv -f sc-design.md architecture/ 2>/dev/null
mv -f staking-client-design.md architecture/ 2>/dev/null

# Move crosschain files
echo "Moving crosschain documentation..."
if [ -d "crosschain-conductor-review" ]; then
    mv -f crosschain-conductor-review/* crosschain/ 2>/dev/null
    rmdir crosschain-conductor-review 2>/dev/null
fi

# Move tool documentation
echo "Moving tool documentation..."
mv -f analyze-tool.md tools/ 2>/dev/null
mv -f debug-tool.md tools/ 2>/dev/null
mv -f simulator-tool.md tools/ 2>/dev/null
mv -f factom-tool.md tools/ 2>/dev/null
mv -f tools-readme.md tools/README.md 2>/dev/null
mv -f a-extract-tool.md tools/ 2>/dev/null
mv -f accumulated-daemon-commands.md tools/ 2>/dev/null
mv -f analyze-*.md tools/ 2>/dev/null

# Move debugging documentation
echo "Moving debugging documentation..."
mv -f debug-*.md debugging/ 2>/dev/null
mv -f TROUBLESHOOTING.md debugging/ 2>/dev/null

# Move setup documentation
echo "Moving setup documentation..."
mv -f devnet-setup.md setup/ 2>/dev/null
mv -f BOOTSTRAP-SERVER.md setup/ 2>/dev/null
if [ -d "configuration" ]; then
    mv -f configuration/* setup/ 2>/dev/null
    rmdir configuration 2>/dev/null
fi

# Move meta documentation
echo "Moving meta documentation..."
mv -f documentation-*.md meta/ 2>/dev/null
mv -f file-naming-*.md meta/ 2>/dev/null
mv -f optimization-summary.md meta/ 2>/dev/null
mv -f design-reviewer-agent.md meta/ 2>/dev/null

# Move network issues
echo "Moving network documentation..."
mv -f PEER-DATABASE-ISSUES.md network/ 2>/dev/null

# Archive old/consolidated files
echo "Archiving old documentation..."
mv -f consolidated-readme.md _archive/ 2>/dev/null
mv -f command-implementation-map.md _archive/ 2>/dev/null

# Create README for each directory
echo "Creating README files..."

cat > architecture/README.md << 'EOF'
# Architecture Documentation

This directory contains system architecture and design documentation for Accumulate.

## Contents

- **network-sync.md** - Network synchronization design
- **P2P-ARCHITECTURE.md** - Peer-to-peer network architecture
- **light-client-design.md** - Light client implementation design
- **sc-design.md** - Smart contract design
- **staking-client-design.md** - Staking client design

## Key Concepts

- Multi-partition architecture (BVNs + Directory Network)
- Cross-partition transaction routing
- Merkle tree based state management
- Consensus mechanisms
EOF

cat > crosschain/README.md << 'EOF'
# CrossChain Conductor Documentation

Documentation for the CrossChain Conductor system that handles cross-partition transactions.

## Key Files

See `internal/core/execute/v2/crosschain/` for implementation:
- `conductor.go` - Main conductor logic
- `types.go` - Data structures
- `recovery.go` - Transaction recovery
- `proof_service.go` - Proof construction/validation

## Features

- Async transaction processing
- Automatic retry with exponential backoff
- Collection proof batching
- Missing transaction recovery
EOF

cat > tools/README.md << 'EOF'
# Tools Documentation

Documentation for various Accumulate tools and utilities.

## Available Tools

- **analyze-tool.md** - Network analysis tool
- **debug-tool.md** - Debugging utilities
- **simulator-tool.md** - Network simulator
- **factom-tool.md** - Factom import tool
- **accumulated-daemon-commands.md** - Daemon command reference

## Usage

Each tool has its own documentation file with usage instructions and examples.
EOF

cat > debugging/README.md << 'EOF'
# Debugging Documentation

Guides and tools for debugging Accumulate networks and applications.

## Contents

- **TROUBLESHOOTING.md** - General troubleshooting guide
- **debug-app-reference.md** - Debug application reference
- **debug-authority-validation.md** - Authority validation debugging
- **debug-lite-client.md** - Lite client debugging
- **debug-snapshot.md** - Snapshot debugging

## Quick Start

Start with TROUBLESHOOTING.md for common issues and solutions.
EOF

cat > setup/README.md << 'EOF'
# Setup Documentation

Guides for setting up and configuring Accumulate networks.

## Contents

- **devnet-setup.md** - Local devnet setup guide
- **BOOTSTRAP-SERVER.md** - Bootstrap server configuration

## Quick Start

For local development, start with devnet-setup.md
EOF

cat > meta/README.md << 'EOF'
# Documentation Metadata

Documentation about the documentation itself - organization, standards, and audits.

## Contents

- Documentation audit reports
- Organization summaries
- File naming standards
- Documentation improvement plans

These files help maintain documentation quality and consistency.
EOF

echo ""
echo "=== Reorganization Summary ==="
echo ""

# Count files in each directory
echo "Files per directory:"
for dir in architecture crosschain tools debugging setup meta network api testing protocol designs _archive; do
    if [ -d "$dir" ]; then
        count=$(find "$dir" -name "*.md" 2>/dev/null | wc -l)
        printf "  %-15s: %d files\n" "$dir" "$count"
    fi
done

echo ""
echo "Root directory files:"
ls -1 *.md 2>/dev/null | head -10

echo ""
echo "=== Reorganization Complete ==="
echo ""
echo "Next steps:"
echo "1. Review AI_INDEX.md for the navigation index"
echo "2. Check each directory's README.md"
echo "3. Update any broken links in documentation"
echo "4. Remove empty directories if needed"
echo ""