# Documentation Reorganization Plan

## Current State
- 35 markdown files scattered in root of docs/
- 24 subdirectories with unclear organization
- No central index or navigation
- Difficult for both humans and AI to navigate

## Proposed Structure

```
docs/
├── AI_INDEX.md                    # AI-optimized navigation index
├── README.md                      # Human-readable documentation guide
│
├── architecture/                  # System architecture & design
│   ├── README.md
│   ├── network-sync.md
│   ├── P2P-ARCHITECTURE.md
│   ├── light-client-design.md
│   ├── sc-design.md
│   └── staking-client-design.md
│
├── crosschain/                    # CrossChain Conductor documentation
│   ├── README.md
│   └── [move crosschain-conductor-review/* here]
│
├── tools/                         # Tool documentation
│   ├── README.md
│   ├── analyze-tool.md
│   ├── debug-tool.md
│   ├── simulator-tool.md
│   ├── factom-tool.md
│   ├── a-extract-tool.md
│   └── accumulated-daemon-commands.md
│
├── debugging/                     # Debugging guides
│   ├── README.md
│   ├── debug-app-reference.md
│   ├── debug-authority-validation.md
│   ├── debug-lite-client.md
│   ├── debug-snapshot.md
│   └── TROUBLESHOOTING.md
│
├── setup/                         # Setup and configuration
│   ├── README.md
│   ├── devnet-setup.md
│   ├── BOOTSTRAP-SERVER.md
│   └── [move configuration/* here]
│
├── api/                          # API documentation
│   ├── README.md
│   └── [existing api/* content]
│
├── testing/                      # Testing documentation
│   ├── README.md
│   └── [existing testing/* content]
│
├── network/                      # Network-specific docs
│   ├── README.md
│   ├── PEER-DATABASE-ISSUES.md
│   └── [existing network/* content]
│
├── protocol/                     # Protocol documentation
│   ├── README.md
│   └── [existing protocol/* content]
│
├── meta/                         # Documentation about documentation
│   ├── README.md
│   ├── documentation-audit-report.md
│   ├── documentation-consolidation-summary.md
│   ├── documentation-organization-summary.md
│   ├── file-naming-standardization-plan.md
│   └── file-naming-standardization-completed.md
│
├── archive/                      # Old/deprecated docs
│   └── [outdated documentation]
│
└── designs/                      # Design documents
    ├── README.md
    └── [existing designs/* content]
```

## AI_INDEX.md Structure

The AI-optimized index will contain:
```markdown
# AI Navigation Index

## Quick Lookup by Topic
- **Architecture**: architecture/README.md
- **CrossChain**: crosschain/README.md
- **Tools**: tools/README.md
- **Debugging**: debugging/README.md
- **Setup**: setup/README.md

## Key Files by Purpose
- **System Design**: architecture/network-sync.md
- **P2P Network**: architecture/P2P-ARCHITECTURE.md
- **CrossChain Conductor**: crosschain/README.md
- **DevNet Setup**: setup/devnet-setup.md
- **Troubleshooting**: debugging/TROUBLESHOOTING.md

## Code Locations
- CrossChain: internal/core/execute/v2/crosschain/
- Load Tests: test/load/sl-load/
- Tools: tools/cmd/
```

## Benefits

1. **Clear Organization**: Each topic in its own folder
2. **Easy Navigation**: Both human and AI friendly
3. **No Clutter**: Root only has index files
4. **Maintainable**: Clear where new docs should go
5. **AI Optimized**: AI_INDEX.md provides quick lookup

## Implementation Steps

1. Create new directory structure
2. Move files to appropriate directories
3. Create README.md for each directory
4. Generate AI_INDEX.md
5. Update root README.md
6. Archive outdated content
7. Update any broken links

## Files to Move

### To architecture/
- network-sync.md
- P2P-ARCHITECTURE.md
- light-client-design.md
- sc-design.md
- staking-client-design.md

### To tools/
- analyze-tool.md
- debug-tool.md
- simulator-tool.md
- factom-tool.md
- tools-readme.md
- a-extract-tool.md
- accumulated-daemon-commands.md
- analyze-*.md files

### To debugging/
- debug-*.md files
- TROUBLESHOOTING.md

### To setup/
- devnet-setup.md
- BOOTSTRAP-SERVER.md
- configuration/* contents

### To meta/
- documentation-*.md files
- file-naming-*.md files
- optimization-summary.md

### To archive/
- consolidated-readme.md (if outdated)
- command-implementation-map.md (if outdated)
- Any other outdated files