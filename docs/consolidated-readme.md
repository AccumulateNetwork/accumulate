# Accumulate Network Documentation - Consolidated Index

This is the complete documentation hub for the Accumulate Network project, with all documentation consolidated into the `/docs` directory for easy discovery and AI assistance.

## 📁 Documentation Structure

### 🏠 Root Documentation (`/docs/root/`)
- [**readme.md**](root/readme.md) - Main project README
- [**changelog.md**](root/changelog.md) - Project changelog
- [**contributing.md**](root/contributing.md) - Contribution guidelines
- [**code-of-conduct.md**](root/code-of-conduct.md) - Code of conduct
- [**bpt-restoration-design.md**](root/bpt-restoration-design.md) - BPT restoration design

### 🔄 Cyclops Validator System (`/docs/cyclops/`)
- [**Cyclops README**](cyclops/README.md) - Complete Cyclops validator system overview
- [**Deployment Scripts**](scripts/README.md) - All deployment scripts in organized directory
- [**Development Deployment Plan**](cyclops/cyclops-development-deployment-plan.md) - 4-phase development deployment
- [**Artifacts Deployment Guide**](cyclops/cyclops-artifacts-deployment-guide.md) - Complete artifacts-based deployment
- [**Deployment Scripts Reference**](cyclops/cyclops-deployment-scripts-reference.md) - Complete deployment automation
- [**Deployment Phases**](cyclops/cyclops-deployment-phases.md) - Detailed deployment phase documentation
- [**TOML Configuration**](cyclops/cyclops-toml-configuration.md) - Configuration templates and generation
- [**Launch Guide**](cyclops/cyclops-launch.md) - Validator launch and startup procedures
- [**Fixes Tracking**](cyclops/cyclops-fixes-tracking.md) - Known issues and fixes tracking
- [**Network JSON Reference**](cyclops/cyclops-network-json-reference.md) - Network configuration reference
- [**Node Startup Guide**](cyclops/cyclops-node-startup-and-bpt-guide.md) - Node startup and BPT procedures
- [**Startup Troubleshooting**](cyclops/cyclops-node-startup-troubleshooting.md) - Startup troubleshooting guide
- [**Cyclops Deployment Design**](cyclops/cyclops-deployment-design.md) - Cyclops deployment design documentation

### 🌐 Network Configuration (`/docs/network/`)
- [**Network Initialization**](network/network-initialization.md) - Complete guide to network initialization
- [**Network JSON Structure**](network/network-json-structure.md) - Network JSON structure and validation
- [**Consensus Creation Workflow**](network/consensus-creation-workflow.md) - Consensus section creation procedures
- [**Network Boot Procedures**](network/network-boot-procedures.md) - Network bootstrap procedures
- [**Mainnet Reference**](network/accumulate-mainnet-reference.md) - Mainnet configuration reference
- [**Network Glossary**](network/accumulate-network-glossary.md) - Network terminology glossary

### 🔧 Technical References (`/docs/technical/`)
- [**BPT Restoration Design**](technical/bpt-restoration-design.md) - BPT restoration strategy and implementation
- [**Snapshot BPT Security Analysis**](technical/snapshot-bpt-security-analysis.md) - Comprehensive BPT security analysis
- [**P2P Key Generation**](technical/p2p-key-generation.md) - P2P key generation procedures
- [**Snapshot Format Overview**](technical/snapshot-format-overview.md) - Snapshot format introduction and concepts
- [**Snapshot Format Structures**](technical/snapshot-format-structures.md) - Data structures and binary format reference
- [**Snapshot Format Sections**](technical/snapshot-format-sections.md) - Section types and organization
- [**Snapshot Format Operations**](technical/snapshot-format-operations.md) - Reading, writing, and processing snapshots
- [**Snapshot Format Combining**](technical/snapshot-format-combining.md) - Algorithms for combining multiple snapshots
- [**Genesis Format**](technical/genesis-format.md) - Genesis document format specification
- [**Record Format**](technical/record-format.md) - Database record format specification
- [**Protocol System**](technical/protocol-system.md) - System protocol documentation
- [**Protocol Transactions**](technical/protocol-transactions.md) - Transaction protocol documentation
- [**Tendermint ABCI Interface**](technical/tendermint-abci-interface.md) - ABCI implementation and consensus integration
- [**Performance Guide**](technical/performance-guide.md) - Performance optimization and troubleshooting guide

### 📡 Client Documentation (`/docs/client/`)
- [**Accumulate Network Clients Guide**](client/accumulate-network-clients-guide.md) - Complete guide to all network clients
- [**Light Client README**](client/light-client-readme.md) - Light client package documentation
- [**Lightclient README**](client/lightclient-readme.md) - Lightclient package documentation
- [**API v2 Client Reference**](client/api-v2-client-reference.md) - API v2 client detailed reference
- [**API v3 JSON-RPC Client Reference**](client/api-v3-jsonrpc-client-reference.md) - API v3 JSON-RPC client detailed reference
- [**API v3 WebSocket Client Reference**](client/api-v3-websocket-client-reference.md) - API v3 WebSocket client detailed reference
- [**API v3 README**](client/api-v3-readme.md) - API v3 architecture and services
- [**Database README**](client/database-readme.md) - Database package documentation

### 🔌 API Documentation (`/docs/api/`)
- [**API Interfaces Reference**](api/api-interfaces-reference.md) - Complete API v2/v3 interfaces and methods reference
- [**Accumulated HTTP Server**](api/accumulated-http-server.md) - HTTP server documentation
- [**API v2 README**](api/api-v2-readme.md) - API v2 implementation documentation
- [**API v3 README**](api/api-v3-readme.md) - API v3 implementation documentation

### 🛠️ Tools Documentation (`/docs/tools/`)
- [**Tools README**](tools/tools-readme.md) - General tools documentation
- [**Debug App Reference**](tools/debug-app-reference.md) - Complete debug application command reference
- [**Analyze Commands**](tools/analyze-commands.md) - Analyze tool command reference
- [**Accumulated Daemon Commands**](tools/accumulated-daemon-commands.md) - Accumulated daemon command reference
- [**Command Implementation Map**](tools/command-implementation-map.md) - Command to implementation mapping
- [**Light Client Design**](tools/light-client-design.md) - Light client design and architecture
- [**Staking Client Design**](tools/staking-client-design.md) - Staking client design and architecture
- [**Analyze Extract Debug**](tools/analyze-extract-debug.md) - Extract debug tool documentation
- [**Analyze Tool**](tools/analyze-tool.md) - Analyze tool documentation
- [**Debug Tool**](tools/debug-tool.md) - Debug tool documentation
- [**Simulator Tool**](tools/simulator-tool.md) - Simulator tool documentation
- [**Factom Tool**](tools/factom-tool.md) - Factom tool documentation

#### Debug Tool Specific (`/docs/tools/debug/`)
- [**Authority Validation**](tools/debug/authority-validation.md) - Authority validation procedures
- [**Lite Client**](tools/debug/lite-client.md) - Lite client documentation
- [**Lite Client Test**](tools/debug/lite-client-test.md) - Lite client testing

### 🔧 Internal Documentation (`/docs/internal/`)
- [**Database README**](internal/database-readme.md) - Database package documentation
- [**Database SMT README**](internal/database-smt-readme.md) - Sparse Merkle Tree implementation
- [**Execute v1 Chain README**](internal/execute-v1-chain-readme.md) - Chain execution v1 documentation
- [**Execute v2 Chain README**](internal/execute-v2-chain-readme.md) - Chain execution v2 documentation
- [**Execute v2 Signing**](internal/execute-v2-signing.md) - Transaction signing v2 documentation
- [**Snapshot**](tools/debug/snapshot.md) - Snapshot operations

#### Analyze Tool Specific (`/docs/tools/analyze/`)
- [**README**](tools/analyze/README.md) - Analyze tool overview
- [**Documentation Complete**](tools/analyze/documentation-complete.md) - Documentation completion status
- [**A Extract Debug**](tools/analyze/a-extract-debug.md) - A Extract debugging
- [**Archive Documentation**](tools/analyze/archive/) - Archived analyze documentation
- [**Deployment Documentation**](tools/analyze/deployment/) - Deployment-specific analyze docs

### 🧪 Test Documentation (`/docs/test/`)
- [**Testing Overview**](test/testing.md) - General testing documentation
- [**AI Guidance**](test/ai-guidance.md) - AI assistance for testing
- [**Unit Tests**](test/unit-tests.md) - Unit testing guidelines
- [**E2E Tests**](test/e2e-tests.md) - End-to-end testing
- [**Simulator Tests**](test/simulator-tests.md) - Simulator testing
- [**Performance Tests**](test/performance-tests.md) - Performance testing
- [**CI/CD**](test/ci-cd.md) - Continuous integration and deployment
- [**Debugging**](test/debugging.md) - Testing debugging procedures
- [**Test Maintenance**](test/test-maintenance.md) - Test maintenance procedures
- [**Test Content**](test/test-content.md) - Test content guidelines

### 🏗️ Internal Documentation (`/docs/internal/`)
- [**API v2 README**](internal/api-v2-readme.md) - Internal API v2 documentation
- [**BSN Notes**](internal/bsn-notes.md) - BSN implementation notes
- [**Execute v1 Chain README**](internal/execute-v1-chain-readme.md) - Execute v1 chain documentation
- [**Execute v2 Chain README**](internal/execute-v2-chain-readme.md) - Execute v2 chain documentation
- [**Execute v2 Signing**](internal/execute-v2-signing.md) - Execute v2 signing procedures
- [**Database SMT README**](internal/database-smt-readme.md) - SMT database documentation

### 📋 Protocol Documentation (`/docs/protocol/`)
- [**System**](protocol/system.md) - System protocol documentation
- [**Transactions**](protocol/transactions.md) - Transaction protocol documentation

### 📜 Scripts Documentation (`/docs/scripts/`)
- [**Scripts README**](scripts/README.md) - Scripts overview and organization
- [**Cyclops Deployment Design**](scripts/CYCLOPS_DEPLOYMENT_DESIGN.md) - Cyclops deployment design

### 🔧 Command Documentation (`/docs/cmd/`)
- [**API Server**](cmd/apiServer.md) - API server documentation

### 🦊 GitLab Documentation (`/docs/gitlab/`)
- [**Default Merge Request Template**](gitlab/Default.md) - Default MR template

## 🔍 Quick Navigation

### By Use Case

#### **Setting Up a Validator**
1. [Cyclops README](cyclops/README.md) - Start here
2. [Development Deployment Plan](cyclops/cyclops-development-deployment-plan.md) - Development setup
3. [Artifacts Deployment Guide](cyclops/cyclops-artifacts-deployment-guide.md) - Production deployment
4. [Launch Guide](cyclops/cyclops-launch.md) - Starting your validator

#### **Network Operations**
1. [Network Initialization](network/network-initialization.md) - Creating networks
2. [Debug App Reference](api/debug-app-reference.md) - Network debugging
3. [Network Boot Procedures](network/network-boot-procedures.md) - Network startup

#### **Client Development**
1. [Accumulate Network Clients Guide](client/accumulate-network-clients-guide.md) - Complete client overview and comparison
2. [Light Client README](client/lightclient-README.md) - Light client usage
3. [API v2 Client Reference](client/api-v2-client-reference.md) - Legacy API v2 client
4. [API v3 JSON-RPC Client Reference](client/api-v3-jsonrpc-client-reference.md) - Modern JSON-RPC client
5. [API v3 WebSocket Client Reference](client/api-v3-websocket-client-reference.md) - Real-time WebSocket client
6. [API v3 README](client/api-v3-README.md) - API v3 architecture
7. [Light Client Design](tools/light-client-design.md) - Design principles
8. [Staking Client Design](tools/staking-client-design.md) - Staking client design

#### **Testing and Development**
1. [Testing Overview](test/testing.md) - Testing guidelines
2. [AI Guidance](test/ai-guidance.md) - AI-assisted development
3. [Debug Tool](tools/debug.md) - Debugging tools
4. [Analyze Tool](tools/analyze.md) - Analysis tools

#### **Contributing**
1. [Contributing Guidelines](root/CONTRIBUTING.md) - How to contribute
2. [Code of Conduct](root/CODE_OF_CONDUCT.md) - Community standards
3. [Test Maintenance](test/test-maintenance.md) - Maintaining tests

## 📊 Documentation Statistics

- **Total Documentation Files**: 113+ markdown files
- **Major Categories**: 12 main documentation categories
- **Tools Covered**: Debug, Analyze, Simulator, Light Client, Staking Client
- **Network Types**: Mainnet, Testnet, Local development
- **Client Types**: Light Client, API v2, API v3 JSON-RPC, API v3 WebSocket (all fully documented)

## 🎯 Documentation Principles

1. **Consolidated**: All documentation in `/docs` directory
2. **Organized**: Clear hierarchical structure by category
3. **Cross-Referenced**: Extensive linking between related documents
4. **AI-Optimized**: Structured for AI assistance and automation
5. **Complete**: Covers all aspects from setup to advanced operations
6. **Maintained**: Regular updates and validation

---

*This consolidated documentation system provides comprehensive coverage of the Accumulate Network ecosystem, optimized for both human developers and AI assistance.*
