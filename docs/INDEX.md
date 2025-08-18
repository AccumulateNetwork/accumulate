# Accumulate Documentation Index

## 🤖 AI-Optimized Documentation
- [AI Context Document](ai-context/AI_CONTEXT.md) - Comprehensive project context for AI systems
- [AI Metadata](ai-context/AI_METADATA.json) - Structured metadata in JSON format
- [AI Semantic Guide](ai-context/AI_SEMANTIC_GUIDE.md) - Semantic markers and patterns for AI understanding

## Quick Links
- [README](../README.md) - Project overview
- [CONTRIBUTING](../CONTRIBUTING.md) - Contribution guidelines
- [CHANGELOG](../CHANGELOG.md) - Release history
- [CODE_OF_CONDUCT](../CODE_OF_CONDUCT.md) - Community guidelines

## Documentation Categories

### 🏗️ Architecture & Design
- **[Design Documentation](design/INDEX.md)** - System architecture and design decisions
  - [CrossChain Conductor Design](design/crosschain/INDEX.md)
  - [Network Synchronization](design/network-sync.md)
  - [Light Client Design](design/light-client-design.md)
  - [Staking Client Design](design/staking-client-design.md)

### 🔌 API Documentation
- **[API Reference](api/INDEX.md)** - Complete API documentation
  - [API v2 Documentation](api/api-v2-readme.md)
  - [API v3 Documentation](api/api-v3-readme.md)
  - [Command Implementation Map](api/command-implementation-map.md)
  - [Accumulated Daemon Commands](api/accumulated-daemon-commands.md)

### 🧪 Testing
- **[Testing Documentation](testing/INDEX.md)** - Test guides and frameworks
  - [DevNet Testing](testing/devnet/INDEX.md)
  - [Load Testing](testing/load/INDEX.md)
  - [Test Coverage Reports](testing/TEST_COVERAGE_REPORT.md)
  - [E2E Testing](testing/e2e-tests.md)

### 🚀 Deployment
- **[Deployment Guides](deployment/INDEX.md)** - Production deployment documentation
  - [Simplified Upgrade Plan](deployment/SIMPLIFIED_UPGRADE_PLAN.md)
  - [TestNet Upgrade Guide](deployment/TESTNET_UPGRADE_WITH_ACCMAN.md)
  - [ACCMAN Commands](deployment/ACCMAN_UPGRADE_COMMANDS.md)

### 🛠️ Tools
- **[Tools Documentation](tools/INDEX.md)** - Development and debugging tools
  - [Analyze Tool](tools/analyze-tool.md)
  - [Debug Tool](tools/debug-tool.md)
  - [Simulator Tool](tools/simulator-tool.md)

### 🌐 Network
- **[Network Documentation](network/INDEX.md)** - Network configuration and management
  - [Network Boot Procedures](network/network-boot-procedures.md)
  - [Network Initialization](network/network-initialization.md)
  - [Port Reference](network/accumulate-port-reference.md)

### 🔧 Technical
- **[Technical Documentation](technical/INDEX.md)** - Technical specifications
  - [Snapshot Format](technical/snapshot-format-overview.md)
  - [Genesis Format](technical/genesis-format.md)
  - [Performance Guide](technical/performance-guide.md)

### 💻 Development
- **[Development Process](development/INDEX.md)** - Development guidelines
  - [Documentation Standards](development/documentation-organization-summary.md)
  - [File Naming Standards](development/file-naming-standardization-plan.md)

### 🏛️ Specific Networks
- **[Cyclops (MainNet)](cyclops/INDEX.md)** - MainNet deployment documentation
- **[Kermit (TestNet)](kermit/INDEX.md)** - TestNet documentation

## Code Structure

### Core Components
- [`internal/core/`](../internal/core/) - Core execution engine
  - [`execute/v2/`](../internal/core/execute/v2/) - Current execution version
  - [`execute/v2/crosschain/`](../internal/core/execute/v2/crosschain/) - CrossChain Conductor

### Protocol
- [`protocol/`](../protocol/) - Protocol definitions
  - [`system.md`](../protocol/system.md) - System protocol documentation
  - [`transactions.md`](../protocol/transactions.md) - Transaction types

### API Implementation
- [`internal/api/v2/`](../internal/api/v2/) - API v2 implementation
- [`pkg/api/v3/`](../pkg/api/v3/) - API v3 implementation

### Database
- [`pkg/database/`](../pkg/database/) - Database layer
- [`internal/database/`](../internal/database/) - Internal database implementation

### Testing
- [`test/`](../test/) - Test suites
  - [`e2e_v2/`](../test/e2e_v2/) - End-to-end tests
  - [`simulator/`](../test/simulator/) - Network simulation tests

### Scripts
- [`scripts/`](../scripts/) - Utility scripts
  - [`devnet/`](../scripts/devnet/) - DevNet management scripts
  - [`testnet/`](../scripts/testnet/) - TestNet management scripts

## Quick Start Guides

1. **[DevNet Setup](testing/devnet/devnet-setup.md)** - Local development network
2. **[Running Tests](testing/readme.md)** - Test execution guide
3. **[API Quick Start](api/api-interfaces-reference.md)** - API usage examples

## Recent Updates

- [Release Notes v1.5.0](release/RELEASE_NOTES_v1.5.0.md)
- [Release Summary](release/RELEASE_SUMMARY.md)
- [Client SDK Improvements](client-sdk-improvements/README.md) - SDK architecture analysis and optimization plan

## Search Documentation

To search across all documentation:
```bash
grep -r "search term" docs/
```

Or use ripgrep for faster searching:
```bash
rg "search term" docs/
```