# Kermit Testnet Documentation

**Last Updated:** 2025-08-05 02:42 CDT  
**Status:** ⚠️ **BVN VALIDATORS DOWN - CONNECTIVITY ISSUE**  
**Version:** v1.4.1

## 📋 Quick Reference

### Essential Information
| Item | Value | Status |
|------|-------|--------|
| **Working API** | `http://kermit-api.accumulate.defidevs.io:16692/v3` | ✅ Operational |
| **Official API** | `https://kermit.accumulatenetwork.io/v3` | ❌ DNS/Proxy Issue |
| **Network Type** | 3 BVN + 1 DN Architecture | ⚠️ **BVN Validators Down** |
| **Consensus** | Directory Network only | ⚠️ **BVNs Not Participating** |
| **Healing Tools** | Updated endpoint | ❌ **Cannot Access BVN Accounts** |

### Test Commands
```bash
# Quick network test
export KERMIT_API="http://kermit-api.accumulate.defidevs.io:16692"
kermit get dn.acme  # ✅ Works (Directory Network)

# Test BVN partitions (currently failing)
kermit get bvn-Chico.acme   # ❌ "no live peers for query:chico"
kermit get bvn-Harpo.acme   # ❌ "no live peers for query:harpo" 
kermit get bvn-Groucho.acme # ❌ "no live peers for query:groucho"

# Verify all partitions
./scripts/kermit/verify-network-status.sh
```

## 🚨 CRITICAL ISSUE: BVN Validators Not Running

**Problem**: Only the Directory Network is accessible. All BVN (Block Validator Network) partitions are showing "no live peers" errors.

**Root Cause**: BVN validator containers/services are not running on the Kermit server.

**Impact**: 
- ❌ Cannot access user accounts (all are in BVN partitions)
- ❌ Cannot perform healing operations on BVN accounts
- ❌ Network is only partially functional (DN only)

**Evidence**:
```bash
# API server is working
curl -X POST -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"node-info","params":{},"id":1}' \
  http://kermit-api.accumulate.defidevs.io:16692/v3
# Returns: {"result":{"peerID":"12D3KooWA8GSa6Tw8kYjrtnkToqnrVAwepjty7N6NkvCCpANUNrQ","network":"Kermit"...}}

# But BVN partitions fail
kermit get bvn-Chico.acme
# Error: dial /acc/Kermit/acc-svc/query:chico: no live peers for query:chico
```

**Solution Required**: 
1. **SSH to Kermit server** and restart BVN validator containers
2. **Run diagnostic script**: `./scripts/kermit/restart-bvn-validators.sh` (on server)
3. **Check Docker containers**: Ensure BVN validators are running
4. **Verify connectivity**: Test P2P connections between validators

**Diagnostic Script Created**: [`restart-bvn-validators.sh`](../../scripts/kermit/restart-bvn-validators.sh)

## 📁 Documentation Structure

### Core Documents
1. **[kermit-fix.md](./kermit-fix.md)** - Complete session history and technical analysis
2. **[kermit-services-fix.md](./kermit-services-fix.md)** - Bootstrap and faucet service configuration
3. **[kermit-https-proxy-fix.md](./kermit-https-proxy-fix.md)** - HTTPS proxy setup for official endpoint
4. **[port-configuration.md](port-configuration.md)** - Definitive port configuration reference with code analysis

### Scripts Directory (`/scripts/kermit/`)
1. **[verify-network-status.sh](../../scripts/kermit/verify-network-status.sh)** - Comprehensive network verification
2. **[diagnose-kermit.sh](../../scripts/kermit/diagnose-kermit.sh)** - Full diagnostic suite
3. **[restart-bvn-validators.sh](../../scripts/kermit/restart-bvn-validators.sh)** - BVN validator restart automation
4. **[validate-port-config.sh](../../scripts/kermit/validate-port-config.sh)** - **NEW** - Port configuration validation
5. **[test-kermit-api.sh](../../scripts/kermit/test-kermit-api.sh)** - API endpoint testing
6. **[check-consensus.sh](../../scripts/kermit/check-consensus.sh)** - Consensus monitoring
7. **[monitor-healing.sh](../../scripts/kermit/monitor-healing.sh)** - Healing process monitoring
8. **[backup-kermit-data.sh](../../scripts/kermit/backup-kermit-data.sh)** - Data backup automation

## 🏗️ Network Architecture

### Topology Overview
```
Kermit Testnet (3 BVN + 1 DN)
├── Directory Network (DN)
│   ├── Consensus: All 3 validators
│   ├── Services: Routing, globals, directory
│   └── Validators: Chico, Harpo, Groucho
├── BVN0: Chico
│   ├── Validator: 604aa762
│   ├── Host: 18.232.151.41
│   └── Ports: 16593 (P2P), 16692 (API)
├── BVN1: Harpo
│   ├── Validator: 3a85548c
│   ├── Host: 52.91.59.159
│   └── Ports: 16593 (P2P), 16692 (API)
└── BVN2: Groucho
    ├── Validator: 9588a5f3
    ├── Host: 54.226.145.213
    └── Ports: 16593 (P2P), 16695 (API)*
    
*Note: Groucho uses non-standard port 16695 for API
```

### API Infrastructure
```
API Access Points:
├── Primary (Working)
│   ├── URL: http://kermit-api.accumulate.defidevs.io:16692/v3
│   ├── Status: ✅ Operational
│   └── Use: All tools and applications
├── Official (Broken)
│   ├── URL: https://kermit.accumulatenetwork.io/v3
│   ├── Status: ❌ DNS/Proxy issue
│   └── Fix: Requires HTTPS proxy setup
└── Direct Node Access
    ├── Standard: http://{node-ip}:16692/v3
    └── Groucho: http://54.226.145.213:16695/v3
```

### Port Configuration

**📋 [Complete Port Reference](port-configuration.md)**

| Service | BVN Port | Formula | Status |
|---------|----------|---------|--------|
| **Accumulate v3 API** | **16695** | 16591+4+100 | ✅ **PRIMARY** |
| **Tendermint RPC** | **16692** | 16591+1+100 | ✅ Available |
| **P2P Network** | **16693** | 16591+2+100 | ✅ Consensus |
| **Prometheus** | **16694** | 16591+3+100 | ✅ Metrics |

**Key Insight**: BVN validators correctly use port **16695** for Accumulate API, not 16692. See [port-configuration.md](port-configuration.md) for complete analysis.

## 🔧 Common Tasks

### For Developers

#### 1. Setting Up Local Environment
```bash
# Clone and build
git clone https://gitlab.com/AccumulateNetwork/accumulate.git
cd accumulate
go build ./cmd/accumulate

# Set working API endpoint
export KERMIT_API="http://kermit-api.accumulate.defidevs.io:16692"

# Test connectivity
go run ./cmd/accumulate get dn.acme -s "$KERMIT_API"
```

#### 2. Running Debug Tools
```bash
# Network status (may show "unknown" for Groucho due to port issue)
go run ./tools/cmd/debug network status Kermit

# Account healing
go run ./tools/cmd/debug heal synthetic-transactions Kermit

# Network scanning
go run ./tools/cmd/debug network scan Kermit
```

#### 3. Testing Accounts
```bash
# Test accounts by partition
export KERMIT_API="http://kermit-api.accumulate.defidevs.io:16692"

# Directory
go run ./cmd/accumulate get dn.acme -s "$KERMIT_API"

# BVN accounts
go run ./cmd/accumulate get bvn-Chico.acme -s "$KERMIT_API"
go run ./cmd/accumulate get bvn-Harpo.acme -s "$KERMIT_API"
go run ./cmd/accumulate get bvn-Groucho.acme -s "$KERMIT_API"

# User accounts
go run ./cmd/accumulate get acc://7f98c50fd1786c23c1bee438317ebf6928b3629936180881/ACME -s "$KERMIT_API"  # $lite2 (Groucho)
go run ./cmd/accumulate get acc://8e139950c56bc26d41c023ec311c77a5fb4b181a4f71d4f1/acme -s "$KERMIT_API"  # $lite (Harpo)
```

### For DevOps/Infrastructure

#### 1. Network Health Check
```bash
# Run comprehensive verification
./scripts/kermit/verify-network-status.sh

# Check specific node
curl -X POST -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"node-info","params":{},"id":1}' \
  http://54.226.145.213:16695/v3
```

#### 2. Service Management
```bash
# Start bootstrap server
./scripts/kermit/start-bootstrap.sh

# Start faucet service  
./scripts/kermit/start-faucet.sh

# Diagnose issues
./scripts/kermit/diagnose-kermit.sh
```

#### 3. Monitoring
```bash
# Check all service ports
for host in 18.232.151.41 52.91.59.159 54.226.145.213; do
  echo "=== $host ==="
  nc -zv $host 16593 16692 16695 26657 2>&1 | grep -E "(succeeded|refused)"
done
```

### For AI Assistants

#### 1. Key Context
- **Network Status**: Fully operational, all nodes healthy
- **Main Issue**: Groucho node API on port 16695 instead of 16692
- **Working Endpoint**: `http://kermit-api.accumulate.defidevs.io:16692/v3`
- **Fixed Issues**: Ed25519 key format, API endpoint configuration

#### 2. Common Debugging Patterns
```bash
# Always use working endpoint
export KERMIT_API="http://kermit-api.accumulate.defidevs.io:16692"

# Check node health via P2P
nc -zv {node-ip} 16593

# Check API service
curl -s http://{node-ip}:16692/v3 || curl -s http://{node-ip}:16695/v3

# Verify account access
go run ./cmd/accumulate get {account} -s "$KERMIT_API"
```

#### 3. Issue Resolution Priorities
1. **API Connectivity**: Always verify working endpoint first
2. **Port Configuration**: Check standard vs actual port usage
3. **P2P Health**: Verify node participation in consensus
4. **Account Access**: Test partition-specific accounts

## 🐛 Known Issues & Solutions

### 1. Groucho Node Port Configuration
**Issue**: API on port 16695 instead of 16692  
**Impact**: Network scanner reports "unknown" version  
**Status**: Cosmetic, doesn't affect functionality  
**Solution**: See [fix-groucho-final.sh](../../scripts/kermit/fix-groucho-final.sh)

### 2. Official HTTPS Endpoint
**Issue**: `https://kermit.accumulatenetwork.io/v3` not responding  
**Impact**: Legacy tools and containers fail  
**Status**: External infrastructure issue  
**Solution**: See [kermit-https-proxy-fix.md](./kermit-https-proxy-fix.md)

### 3. Ed25519 Key Format (RESOLVED)
**Issue**: Node panic with 64-byte Ed25519 keys  
**Status**: ✅ Fixed in codebase  
**Solution**: Proper seed extraction logic added

## 📊 Network Status

### Current Health (2025-08-05 02:42 CDT)
| Component | Status | Notes |
|-----------|--------|-------|
| **Directory Network** | ✅ Healthy | API server responding, DN accounts accessible |
| **BVN0 (Chico)** | ❌ **DOWN** | "no live peers for query:chico" |
| **BVN1 (Harpo)** | ❌ **DOWN** | "no live peers for query:harpo" |
| **BVN2 (Groucho)** | ❌ **DOWN** | "no live peers for query:groucho" |
| **P2P Connectivity** | ⚠️ **PARTIAL** | API server connected, BVN validators missing |
| **Consensus** | ⚠️ **DN ONLY** | Directory Network consensus working |
| **API Services** | ✅ Working | Primary endpoint operational |
| **Account Access** | ❌ **BVN ACCOUNTS INACCESSIBLE** | Only DN accounts work |

### Performance Metrics (DEGRADED)
- **Block Time**: DN only (BVN blocks not being produced)
- **Transaction Processing**: DN only (BVN transactions failing)
- **Healing Operations**: ❌ **FAILING** (cannot access BVN accounts)
- **Account Queries**: DN sub-second, BVN timeout/error

## 🔄 Maintenance Procedures

### Regular Health Checks
```bash
# Daily verification
./scripts/kermit/verify-network-status.sh

# Weekly deep scan
./scripts/kermit/diagnose-kermit.sh

# Monitor logs
docker logs kermit-api-container
```

### Emergency Procedures
1. **API Outage**: Switch to direct node access
2. **Node Failure**: Check P2P connectivity and restart
3. **Consensus Issues**: Verify all validators are online
4. **Account Access**: Test partition routing

## 📞 Support Information

### Quick Diagnostics
```bash
# Network connectivity
ping kermit-api.accumulate.defidevs.io

# API health
curl http://kermit-api.accumulate.defidevs.io:16692/v3

# Node status
curl -X POST -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"node-info","params":{},"id":1}' \
  http://kermit-api.accumulate.defidevs.io:16692/v3
```

### Log Locations
- **API Server**: Docker container logs
- **Validator Nodes**: `/opt/accumulate/*/logs/`
- **Debug Tools**: Console output

### Key Files
- **Configuration**: `accumulate-kermit.toml`
- **Keys**: `/opt/accumulate/*/keys/`
- **Database**: `/opt/accumulate/*/database/`

---

## 📝 Change Log

### 2025-08-05
- ✅ Complete network analysis and documentation
- ✅ Fixed API endpoint configuration
- ✅ Resolved Ed25519 key format issue
- ✅ Identified and documented Groucho port configuration
- ✅ Created comprehensive diagnostic scripts
- ✅ Verified all network partitions operational

### Previous Sessions
- Fixed BVN connectivity issues
- Resolved protocol version mismatches
- Updated healing tool endpoints
- Created service startup scripts

---

*This documentation is maintained as part of the Accumulate Network Kermit testnet operations. For updates or issues, refer to the GitLab repository.*
