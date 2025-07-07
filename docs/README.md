# Accumulate Network Documentation

This is the comprehensive documentation hub for the Accumulate Network project, optimized for AI assistance and developer productivity.

## 📚 Documentation Structure

### Core Documentation
- [**network-initialization**](network-initialization.md) - Complete guide to network initialization and genesis creation
- [**p2p-key-generation**](p2p-key-generation.md) - P2P key generation procedures
- [**debug-app-reference**](debug-app-reference.md) - Complete debug application command reference

### Cyclops Validator System
- [**cyclops-preparation**](cyclops/cyclops-preparation.md) - Cyclops validator preparation workflow
- [**cyclops-deployment**](cyclops/cyclops-deployment.md) - Cyclops validator deployment procedures  
- [**cyclops-launch**](cyclops/cyclops-launch.md) - Cyclops validator launch procedures
- [**cyclops-automation**](cyclops/cyclops-automation.md) - Complete automation system documentation
- [**cyclops-deployment-design**](cyclops/cyclops-deployment-design.md) - Cyclops deployment design and architecture

### Network Configuration
- [**network-json-structure**](network-json-structure.md) - Network JSON structure and validation
- [**consensus-creation-workflow**](consensus-creation-workflow.md) - Consensus section creation procedures
- [**network-boot-procedures**](network-boot-procedures.md) - Network bootstrap procedures

### Technical References
- [**snapshot-format**](technical/snapshot-format.md) - Snapshot file format specification
- [**genesis-format**](technical/genesis-format.md) - Genesis document format specification
- [**record-format**](technical/record-format.md) - Database record format specification
- [**extract-implementation**](technical/extract-implementation-status.md) - Extract command implementation status

### API Documentation
- [**analyze-commands**](api/analyze-commands.md) - Analyze tool command reference
- [**accumulated-daemon-commands**](api/accumulated-daemon-commands.md) - Accumulated daemon command reference
- [**command-implementation-map**](api/command-implementation-map.md) - Command to implementation mapping

### Network References
- [**accumulate-mainnet-reference**](network/accumulate-mainnet-reference.md) - Mainnet configuration reference
- [**accumulate-network-glossary**](network/accumulate-network-glossary.md) - Network terminology glossary

### Tools and Utilities
- [**a-extract-tool**](tools/a-extract-tool.md) - A_Extract tool documentation
- [**sc-design**](tools/sc-design.md) - Snapshot Collection design documentation

## 🔗 Cross-References

### By Topic

#### Network Initialization
- [network-initialization.md](network-initialization.md) ← Primary reference
- [cyclops/cyclops-preparation.md](cyclops/cyclops-preparation.md) ← Validator-specific procedures
- [consensus-creation-workflow.md](consensus-creation-workflow.md) ← Consensus generation
- [network-json-structure.md](network-json-structure.md) ← Configuration format

#### Snapshot Management
- [snapshot-format.md](technical/snapshot-format.md) ← Format specification
- [a-extract-tool.md](tools/a-extract-tool.md) ← Extraction procedures
- [debug-app-reference.md](debug-app-reference.md) ← Debug commands

#### Validator Operations
- [cyclops-preparation.md](cyclops-preparation.md) ← Preparation phase
- [cyclops-deployment.md](cyclops-deployment.md) ← Deployment phase
- [cyclops-launch.md](cyclops-launch.md) ← Launch phase
- [cyclops-automation.md](cyclops-automation.md) ← Complete automation
- [consensus-generation-fix.md](cyclops/consensus-generation-fix.md) ← CometBFT format conversion
- [consensus-code-changes.md](cyclops/consensus-code-changes.md) ← Technical implementation
- [examples/](cyclops/examples/) ← Example consensus JSON files

#### Development Tools
- [debug-app-reference.md](debug-app-reference.md) ← Debug application
- [analyze-commands.md](api/analyze-commands.md) ← Analysis tools
- [accumulated-daemon-commands.md](api/accumulated-daemon-commands.md) ← Daemon operations

### By Use Case

#### **Setting up a new network**
1. [network-initialization.md](network-initialization.md) - Overall process
2. [network-json-structure.md](network-json-structure.md) - Configuration format
3. [consensus-creation-workflow.md](consensus-creation-workflow.md) - Consensus setup
4. [consensus-generation-fix.md](cyclops/consensus-generation-fix.md) - CometBFT format conversion
5. [p2p-key-generation.md](p2p-key-generation.md) - Key generation

#### **Deploying Cyclops validators**
1. [cyclops-preparation.md](cyclops-preparation.md) - Preparation phase
2. [cyclops-deployment.md](cyclops-deployment.md) - Deployment phase
3. [cyclops-launch.md](cyclops-launch.md) - Launch phase
4. [cyclops-automation.md](cyclops-automation.md) - Automation scripts

#### **Debugging network issues**
1. [debug-app-reference.md](debug-app-reference.md) - Debug commands
2. [network-boot-procedures.md](network-boot-procedures.md) - Boot troubleshooting
3. [extract-implementation-status.md](technical/extract-implementation-status.md) - Known issues

#### **Working with snapshots**
1. [snapshot-format.md](technical/snapshot-format.md) - Format details
2. [a-extract-tool.md](tools/a-extract-tool.md) - Extraction tools
3. [debug-app-reference.md](debug-app-reference.md) - Debug snapshot commands

## 🤖 AI Assistant Guidelines

### For AI Systems
- **Primary References**: Always check the main topic files first (network-initialization.md, cyclops-preparation.md, debug-app-reference.md)
- **Cross-Reference Pattern**: Follow the "See also" sections in each document
- **Command References**: Use api/ directory for exact command syntax and flags
- **Technical Details**: Refer to technical/ directory for format specifications
- **Troubleshooting**: Check both main docs and debug-app-reference.md for solutions

### For Developers
- **Quick Start**: Begin with README.md in each section
- **Complete Workflows**: Follow the cyclops-* series for end-to-end procedures
- **Command Reference**: Use api/ docs for exact syntax
- **Troubleshooting**: Debug-app-reference.md contains comprehensive troubleshooting

## 📋 Documentation Standards

### File Naming Convention
- Use lowercase with dashes: `network-initialization.md`
- Descriptive names: `cyclops-preparation.md` not `prep.md`
- Consistent prefixes: `cyclops-*` for validator docs, `network-*` for network docs

### Cross-Reference Format
```markdown
## See Also
- [Related Topic](related-topic.md) - Brief description
- [Another Topic](another-topic.md) - Brief description

## Related Commands
- `command-name` - See [command-reference.md](api/command-reference.md)
```

### Section Structure
1. **Overview** - What this document covers
2. **Prerequisites** - What you need before starting
3. **Procedures** - Step-by-step instructions
4. **Troubleshooting** - Common issues and solutions
5. **See Also** - Cross-references to related documentation

## 🔄 Last Updated
This documentation index was last updated: 2025-07-06

---
*This documentation is optimized for AI assistance and developer productivity. All files follow consistent naming conventions and cross-referencing patterns.*
