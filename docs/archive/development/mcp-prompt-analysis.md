# Accumulate MCP - Prompt Analysis

**Repository**: accumulate
**MCP Server Location**: `/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/mcp`
**Purpose**: MCP server for Accumulate blockchain - follower deployment and management
**Started**: 2025-11-23

---

## Phase 1: Repository Analysis ✅

### 1.1 Repository Purpose

**What is this repository?**
- Accumulate blockchain protocol implementation
- Full-featured MCP server for blockchain operations
- Focus on follower node deployment and management
- Support for wallet operations, queries, and transactions

**Users**:
- Node operators deploying follower nodes
- Developers building on Accumulate
- Operators managing blockchain infrastructure
- AI assistants (via Claude Desktop)

**Main Workflows**:
1. **Deploy follower nodes** from database snapshots
2. **Manage wallets** (init, unlock, generate keys)
3. **Query blockchain** (accounts, transactions, chains)
4. **Submit transactions** (send tokens, create accounts)
5. **Monitor nodes** (status, sync, health)
6. **Historical analysis** (database queries, merkle proofs)

### 1.2 MCP Tools Inventory

**Total Tools**: 40+ implemented tools

#### Wallet Management (7 tools)
| Tool | Purpose |
|------|---------|
| `wallet_init` | Initialize new wallet |
| `wallet_vault_open` | Unlock vault |
| `wallet_vault_lock` | Lock vault |
| `wallet_generate_key` | Generate key pair |
| `wallet_list_keys` | List all keys |
| `wallet_set_network` | Set network (mainnet/testnet/devnet) |
| `wallet_get_status` | Get wallet status |

#### Query Operations (11+ tools)
| Tool | Purpose |
|------|---------|
| `accumulate_query_account` | Query account details |
| `accumulate_query_tx` | Query transaction |
| `accumulate_query_chain` | Query chain entries |
| `accumulate_query_data` | Query data entries |
| `accumulate_query_directory` | Query directory |
| `accumulate_query_pending` | Query pending transactions |
| `accumulate_query_keybook` | Query keybook |
| `accumulate_query_keypage` | Query keypage |
| `accumulate_query_minor_block` | Query minor block |
| `accumulate_query_major_block` | Query major block |
| `accumulate_search_public_key` | Search by public key |

#### Transaction Operations (15+ tools)
| Tool | Purpose |
|------|---------|
| `accumulate_send_tokens` | Send ACME tokens |
| `accumulate_create_lite_account` | Create lite account |
| `accumulate_create_adi` | Create ADI |
| `accumulate_create_data_account` | Create data account |
| `accumulate_create_token_account` | Create token account |
| `accumulate_create_keypage` | Create keypage |
| `accumulate_create_keybook` | Create keybook |
| `accumulate_create_token` | Create custom token |
| `accumulate_write_data` | Write data to account |
| `accumulate_generate_key` | Generate key pair |
| `accumulate_add_credits` | Add credits |
| `accumulate_update_keypage` | Update keypage |
| `accumulate_update_account_auth` | Update account authority |
| `accumulate_issue_tokens` | Issue tokens |
| `accumulate_burn_tokens` | Burn tokens |

#### Follower Node Management (3+ tools)
| Tool | Purpose |
|------|---------|
| `accumulate_init_follower` | Initialize follower from snapshots |
| `accumulate_run_follower` | Start follower node |
| `accumulate_follower_status` | Check follower status |

#### Network & Status (4 tools)
| Tool | Purpose |
|------|---------|
| `accumulate_node_info` | Get node information |
| `accumulate_network_status` | Get network status |
| `accumulate_consensus_status` | Get consensus status |
| `accumulate_metrics` | Get node metrics |

#### Additional Tools
- `accumulate_search_public_key_hash` - Search by key hash
- `accumulate_search_anchor` - Search by anchor
- `accumulate_faucet` - Get testnet tokens
- Database tools (batch extract, fulldb build, fullscan)
- Snapshot restore tools

### 1.3 Common Questions/Tasks

From documentation review:

**Follower Deployment**:
- "How do I deploy a follower node from snapshots?"
- "What database snapshots do I need?"
- "How do I configure network peers?"
- "How do I verify my follower is syncing?"
- "How do I monitor follower health?"

**Wallet Operations**:
- "How do I create a new wallet?"
- "How do I generate keys?"
- "How do I switch networks?"
- "What keys do I have?"

**Account Management**:
- "How do I check my account balance?"
- "How do I send tokens?"
- "How do I create an ADI?"
- "How do I query transaction history?"

**Troubleshooting**:
- "Why isn't my follower syncing?"
- "How do I check peer connections?"
- "How do I verify database integrity?"
- "What's my node's current block height?"

### 1.4 Key Documentation

**Follower Deployment**:
- `FOLLOWER_SETUP_GUIDE.md` - Step-by-step follower setup
- `FOLLOWER_DOCKER_GUIDE.md` - Docker deployment
- `MCP_ARCHITECTURE.md` - MCP server architecture
- `BOOTSTRAP_ARCHITECTURE.md` - Bootstrap process

**Database**:
- `MCP_DATABASE_ACCESS_INVESTIGATION.md` - Database access
- `database_health_report.md` - Database health
- `snapshot_restore_readme.md` - Snapshot restoration

**Configuration**:
- `CONFIG_VALIDATION.md` - Configuration validation
- `MCP_DEPLOYMENT_ISSUES.md` - Common issues

**Integration**:
- `ACCMAN_INTEGRATION_GUIDE.md` - Accman integration
- `accman_snapshot_restore_review.md` - Snapshot review

### 1.5 User Pain Points

Based on documentation and issue tracking:

1. **Multi-step follower deployment**
   - Requires database snapshots, configuration, network setup
   - 10+ manual steps currently
   - Easy to misconfigure

2. **Database snapshot selection**
   - Which snapshots to use?
   - How recent do they need to be?
   - Where to get them?

3. **Network configuration**
   - Finding healthy peers
   - Configuring bootstrap peers
   - Setting up seed proxies

4. **Sync verification**
   - Is my follower syncing?
   - How far behind am I?
   - When will sync complete?

5. **Troubleshooting**
   - Follower not connecting to peers
   - Database errors
   - Configuration issues

---

## Phase 2: Workflow Identification (In Progress)

### Primary Workflows

#### 1. Deploy New Follower Node ⭐ (HIGH PRIORITY)
**User Goal**: Get a working follower node from scratch

**Steps**:
1. Check prerequisites (disk space, network)
2. Obtain database snapshots (DN + BVN)
3. Initialize follower configuration
4. Configure network peers
5. Start follower node
6. Verify synchronization
7. Monitor ongoing health

**Tools Needed**:
- `accumulate_init_follower`
- `accumulate_run_follower`
- `accumulate_follower_status`
- `accumulate_node_info`
- `accumulate_network_status`

**Pain Points**:
- Manual, error-prone process
- Requires deep knowledge
- 10+ steps across multiple tools

**Prompt Opportunity**: ✅ **HIGH VALUE**
- Combines 5+ tools
- Reduces 10+ steps to 1 prompt
- Encodes best practices
- Handles common errors

---

#### 2. Monitor Follower Health ⭐
**User Goal**: Check if follower is healthy and syncing

**Steps**:
1. Check process status
2. Get current block height
3. Compare to network height
4. Check peer count
5. Check sync status
6. Review recent logs

**Tools Needed**:
- `accumulate_follower_status`
- `accumulate_node_info`
- `accumulate_network_status`
- `accumulate_consensus_status`

**Prompt Opportunity**: ✅ **HIGH VALUE**

---

#### 3. Troubleshoot Sync Issues ⭐
**User Goal**: Diagnose why follower isn't syncing

**Steps**:
1. Check process is running
2. Verify peer connections
3. Check network connectivity
4. Review database health
5. Check logs for errors
6. Verify configuration

**Tools Needed**:
- `accumulate_follower_status`
- `accumulate_node_info`
- `accumulate_network_status`
- Database diagnostic tools

**Prompt Opportunity**: ✅ **HIGH VALUE**

---

#### 4. Upgrade Follower Node
**User Goal**: Upgrade follower to new version

**Steps**:
1. Stop follower
2. Backup current state
3. Download new binary
4. Verify binary
5. Restart follower
6. Verify sync resumes

**Tools Needed**:
- `accumulate_follower_status`
- Process management
- File operations

**Prompt Opportunity**: ✅ **MEDIUM VALUE**

---

#### 5. Wallet Setup for Development
**User Goal**: Set up wallet for testing/development

**Steps**:
1. Initialize wallet
2. Create vault
3. Generate keys
4. Set network (testnet/devnet)
5. Get faucet tokens (testnet)
6. Verify wallet status

**Tools Needed**:
- `wallet_init`
- `wallet_vault_open`
- `wallet_generate_key`
- `wallet_set_network`
- `accumulate_faucet`
- `wallet_get_status`

**Prompt Opportunity**: ✅ **MEDIUM VALUE**

---

#### 6. Query Account History
**User Goal**: Get complete history for an account

**Steps**:
1. Query account details
2. Query main chain
3. Query signature chain
4. Parse transaction history
5. Format results

**Tools Needed**:
- `accumulate_query_account`
- `accumulate_query_chain`
- `accumulate_query_tx`

**Prompt Opportunity**: ✅ **MEDIUM VALUE**

---

#### 7. Create and Fund New ADI
**User Goal**: Set up a new ADI identity

**Steps**:
1. Generate key
2. Create lite account
3. Fund lite account
4. Create ADI
5. Create token account
6. Verify setup

**Tools Needed**:
- `wallet_generate_key`
- `accumulate_create_lite_account`
- `accumulate_send_tokens` (or faucet)
- `accumulate_create_adi`
- `accumulate_create_token_account`
- `accumulate_query_account`

**Prompt Opportunity**: ✅ **MEDIUM VALUE**

---

#### 8. Backup Follower Data
**User Goal**: Create backup of follower state

**Steps**:
1. Stop follower (optional)
2. Snapshot databases
3. Backup configuration
4. Verify backup
5. Restart follower

**Tools Needed**:
- `accumulate_follower_status`
- Snapshot tools
- File operations

**Prompt Opportunity**: ✅ **LOW-MEDIUM VALUE**

---

### Workflow Priority Matrix

| Workflow | Complexity | Frequency | Value | Priority |
|----------|------------|-----------|-------|----------|
| Deploy Follower | Very High | Low | Very High | 🔴 **CRITICAL** |
| Monitor Health | Low | Very High | High | 🔴 **HIGH** |
| Troubleshoot Sync | High | Medium | High | 🔴 **HIGH** |
| Wallet Setup | Medium | High | Medium | 🟡 **MEDIUM** |
| Query History | Medium | High | Medium | 🟡 **MEDIUM** |
| Create ADI | Medium | Medium | Medium | 🟡 **MEDIUM** |
| Upgrade Node | High | Low | Medium | 🟢 **LOW** |
| Backup Data | Medium | Low | Low | 🟢 **LOW** |

---

## Recommended Prompts (Phase 3 Preview)

Based on Phase 1 & 2 analysis:

### High Priority (Must Have)
1. **`deploy-follower-node`** ⭐⭐⭐
   - Complete follower deployment workflow
   - Combines 5+ tools
   - Highest value/complexity ratio

2. **`monitor-follower-health`** ⭐⭐⭐
   - Quick health check
   - Used frequently
   - Clear success criteria

3. **`troubleshoot-follower-sync`** ⭐⭐⭐
   - Diagnostic workflow
   - If/then logic
   - Common pain point

### Medium Priority (Should Have)
4. **`setup-dev-wallet`**
   - Development environment setup
   - Testnet/devnet focused

5. **`query-account-complete`**
   - Full account history
   - Combines multiple queries

6. **`create-adi-complete`**
   - End-to-end ADI creation
   - Includes funding

### Low Priority (Nice to Have)
7. **`upgrade-follower`**
   - Node upgrade workflow

8. **`backup-follower`**
   - Backup procedures

9. **`restore-from-snapshot`**
   - Disaster recovery

### Quick Status Prompts
10. **`quick-node-status`**
    - Fast overview
    - <15 lines output

11. **`quick-wallet-status`**
    - Wallet summary

---

## Next Steps

- [x] Phase 1: Repository Analysis
- [ ] Phase 2: Complete workflow mapping
- [ ] Phase 3: Design prompts (focus on top 3-5)
- [ ] Phase 4: Implement prompts
- [ ] Phase 5: Test with real deployments
- [ ] Phase 6: Document and deploy
