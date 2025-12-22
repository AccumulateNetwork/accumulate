# Accumulate MCP - Prompts Design Specification

**Based on**: PROMPT_ANALYSIS.md
**Focus**: Follower node deployment and management
**Target**: 5-8 high-value prompts

---

## Design Principle

Each prompt should:
- Combine 2+ tools into coherent workflow
- Encode best practices from FOLLOWER_SETUP_GUIDE.md
- Handle common failure modes
- Provide clear validation steps
- Link to related prompts

---

## Prompt 1: deploy-follower-node ⭐⭐⭐

**Priority**: CRITICAL
**Combines**: 5+ tools
**Use Case**: Deploy a new Accumulate follower node from database snapshots

### Specification

```go
{
    Name:        "deploy-follower-node",
    Description: "Complete workflow for deploying an Accumulate follower node from database snapshots",
    Arguments: []PromptArgument{
        {
            Name:        "dn_database",
            Description: "Path to Directory Network database snapshot",
            Required:    true,
        },
        {
            Name:        "bvn_database",
            Description: "Path to Block Validation Network database snapshot",
            Required:    true,
        },
        {
            Name:        "work_dir",
            Description: "Directory for follower configuration and data",
            Required:    true,
        },
        {
            Name:        "peer_url",
            Description: "Peer BVN URL to connect to (e.g., tcp://peer.example.com:16691)",
            Required:    false,
        },
        {
            Name:        "seed_proxy",
            Description: "Seed proxy URL for network configuration",
            Required:    false,
        },
        {
            Name:        "public_ip",
            Description: "Follower's public IP address",
            Required:    false,
        },
    },
}
```

### Template Structure

```markdown
Deploy Accumulate follower node to: {work_dir}

**Database Snapshots:**
- DN: {dn_database}
- BVN: {bvn_database}

**Prerequisites Check:**
1. Verify database snapshots exist
   - Check {dn_database} is valid directory
   - Check {bvn_database} is valid directory
   - Estimate disk space needed: ~2-4GB

2. Verify network configuration
   - Peer URL: {peer_url or "Not provided"}
   - Seed Proxy: {seed_proxy or "Not provided"}
   - At least ONE must be provided

3. Check system requirements
   - Disk space: 100+ GB free
   - Memory: 8+ GB RAM
   - CPU: 4+ cores recommended
   - Network: Ports 16591-16593 open

**Initialization:**

Step 1: Use `accumulate_init_follower` with:
```json
{
  "dn_database": "{dn_database}",
  "bvn_database": "{bvn_database}",
  "work_dir": "{work_dir}",
  "peer_url": "{peer_url}",
  "seed_proxy": "{seed_proxy}",
  "public_ip": "{public_ip}"
}
```

**Expected Output:**
- Status: "initialized"
- DN copied to: {work_dir}/dnn
- BVN copied to: {work_dir}/bvnn
- Configuration created in {work_dir}

**Validation After Init:**
- [ ] Directory {work_dir} exists
- [ ] Directory {work_dir}/dnn exists and contains database
- [ ] Directory {work_dir}/bvnn exists and contains database
- [ ] Configuration files created (accumulated.toml, etc.)

**Start Follower:**

Step 2: Use `accumulate_run_follower` with:
```json
{
  "work_dir": "{work_dir}",
  "background": true
}
```

**Expected Output:**
- Status: "started"
- PID: [process ID]
- Message: "Follower started in background"

**Verify Startup:**

Step 3: Wait 30-60 seconds, then use `accumulate_follower_status`:
```json
{
  "work_dir": "{work_dir}"
}
```

Check for:
- [ ] Process is running
- [ ] No critical errors in recent logs
- [ ] Node is initializing

**Monitor Synchronization:**

Step 4: Use `accumulate_node_info` to check sync status:
```json
{
  "network": "follower endpoint or mainnet"
}
```

Then use `accumulate_network_status` to get network height

**Synchronization Validation:**
- [ ] Node is running (process check)
- [ ] Peers connected (should have 3+ peers within 5 minutes)
- [ ] Syncing started (current_block increasing)
- [ ] No database errors in logs

**Expected Timeline:**
- Startup: 1-2 minutes
- First peers: 2-5 minutes
- Sync begins: 5-10 minutes
- Full sync: 2-24 hours (depends on how old snapshots are)

**Troubleshooting:**

If initialization fails:
  ❌ "source database not found"
     → Verify database paths are correct and absolute
     → Check databases exist: ls -la {dn_database} {bvn_database}

  ❌ "must provide either peer_url or seed_proxy"
     → Provide at least one connection method
     → For mainnet, use: tcp://mainnet.accumulate.defidevs.io:16691

If startup fails:
  ❌ "DN directory not found"
     → Run accumulate_init_follower first
     → Check {work_dir}/dnn exists

  ❌ Process dies immediately
     → Check logs in {work_dir}/accumulated.log
     → Use troubleshoot-follower-sync prompt

If no peers connecting:
  ❌ Peer count = 0 after 5 minutes
     → Verify firewall allows ports 16591-16593
     → Check peer_url is reachable: telnet peer.host 16691
     → Try alternative peer URL
     → Use prompt: troubleshoot-follower-sync

If sync not starting:
  ❌ Current block not increasing
     → Check peers are connected (need 3+)
     → Review logs for errors
     → Verify databases are from recent snapshot
     → Use prompt: troubleshoot-follower-sync

**Next Steps:**

After successful deployment:
- Monitor sync progress: Use `monitor-follower-health` prompt (every 15 min)
- Check when fully synced: Compare local height vs network height
- Set up monitoring: Configure alerting for node health
- Plan backup: Use `backup-follower` prompt weekly

**Quick Verification Command Sequence:**
1. accumulate_follower_status - Process running?
2. accumulate_node_info - Peer count & block height
3. accumulate_network_status - Network height (compare)
4. Review logs - Any errors?

**Reference Files:**
- Setup Guide: FOLLOWER_SETUP_GUIDE.md
- Troubleshooting: TROUBLESHOOTING.md
- Architecture: BOOTSTRAP_ARCHITECTURE.md

**Related Prompts:**
- `monitor-follower-health` - Ongoing monitoring
- `troubleshoot-follower-sync` - Sync issues
- `upgrade-follower` - Version upgrades
- `backup-follower` - Data backup
```

---

## Prompt 2: monitor-follower-health ⭐⭐⭐

**Priority**: HIGH
**Combines**: 3 tools
**Use Case**: Quick health check for running follower

### Specification

```go
{
    Name:        "monitor-follower-health",
    Description: "Monitor health and sync status of Accumulate follower node",
    Arguments: []PromptArgument{
        {
            Name:        "work_dir",
            Description: "Follower working directory",
            Required:    false,
        },
        {
            Name:        "endpoint",
            Description: "Follower API endpoint (if different from default)",
            Required:    false,
        },
    },
}
```

### Template Structure

```markdown
Monitor Accumulate follower health

**Quick Health Check:**

Step 1: Check process status
Use `accumulate_follower_status`:
```json
{
  "work_dir": "{work_dir or ~/.accumulate/follower}"
}
```

**Process Health:**
- [ ] Running: YES/NO
- [ ] PID: [number]
- [ ] Uptime: [duration]

Step 2: Get node information
Use `accumulate_node_info`:
```json
{
  "network": "{endpoint or mainnet}"
}
```

**Node Metrics:**
- Current Block: [height]
- Peer Count: [count]
- Sync Status: [syncing/synced/behind]
- Last Block Time: [timestamp]

Step 3: Get network status (for comparison)
Use `accumulate_network_status`:
```json
{
  "network": "mainnet"
}
```

**Network Comparison:**
- Network Height: [height]
- Follower Height: [height from step 2]
- Blocks Behind: [difference]
- Catch-up Rate: [blocks/minute if syncing]

**Health Status Summary:**

✅ **HEALTHY** if:
- Process running
- Peers ≥ 3
- Blocks behind < 100 OR syncing actively
- No critical errors in logs

⚠️ **WARNING** if:
- Peers < 3 but > 0
- Blocks behind 100-1000
- Slow catch-up rate

❌ **UNHEALTHY** if:
- Process not running
- Peers = 0
- Blocks behind > 1000 and not catching up
- Critical errors in logs

**Recommended Actions:**

If HEALTHY:
  ✅ Continue monitoring
  ✅ Check again in 15-30 minutes

If WARNING:
  ⚠️ Investigate peer connections
  ⚠️ Check network connectivity
  ⚠️ Monitor for 10-15 minutes
  ⚠️ If persists, use `troubleshoot-follower-sync`

If UNHEALTHY:
  ❌ Use `troubleshoot-follower-sync` prompt immediately
  ❌ Review recent logs
  ❌ Consider restart if stuck

**Related Prompts:**
- `troubleshoot-follower-sync` - If issues detected
- `deploy-follower-node` - Initial deployment
- `quick-node-status` - Even faster check
```

---

## Prompt 3: troubleshoot-follower-sync ⭐⭐⭐

**Priority**: HIGH
**Combines**: 4+ tools + diagnostics
**Use Case**: Diagnose and fix follower synchronization issues

### Specification

```go
{
    Name:        "troubleshoot-follower-sync",
    Description: "Diagnose and resolve follower node synchronization issues",
    Arguments: []PromptArgument{
        {
            Name:        "work_dir",
            Description: "Follower working directory",
            Required:    false,
        },
        {
            Name:        "symptom",
            Description: "Observed issue: no_peers, not_syncing, slow_sync, or crashed",
            Required:    false,
        },
    },
}
```

### Template Structure

```markdown
Troubleshoot Accumulate Follower Sync Issues
Symptom: {symptom or "general"}

**Diagnostic Steps:**

**1. Process Check**
Use `accumulate_follower_status`
- Is process running? YES/NO
- If NO → Process crashed or not started
- If YES → Continue diagnostics

**2. Peer Connection Check**
Use `accumulate_node_info`
- Peer count: [number]
- If 0 peers → Network connectivity issue
- If 1-2 peers → Degraded but may work
- If 3+ peers → Peers OK

**3. Block Height Check**
Use `accumulate_node_info` + `accumulate_network_status`
- Local height: [number]
- Network height: [number]
- Behind by: [difference]
- If not advancing → Sync stalled

**4. Log Review**
Check recent logs for:
- Database errors
- Network errors
- Consensus errors
- Panic/crash messages

**Issue-Specific Troubleshooting:**

### SYMPTOM: no_peers (Peer count = 0)

**Likely Causes:**
1. Firewall blocking ports
2. Incorrect peer URL
3. Network connectivity
4. Peer is down

**Resolution Steps:**

A. Verify network connectivity
   ```bash
   # Check if peer URL is reachable
   telnet mainnet.accumulate.defidevs.io 16691
   ```
   If fails → Network/firewall issue

B. Check firewall rules
   - Ports 16591-16593 must be open
   - Both inbound and outbound
   - Check: `sudo ufw status` or `iptables -L`

C. Try alternative peer
   - Use `accumulate_init_follower` with different peer_url
   - Mainnet peers:
     - tcp://mainnet.accumulate.defidevs.io:16691
     - [Add more known good peers]

D. Verify configuration
   - Check {work_dir}/accumulated.toml
   - Verify peer settings correct

**Fix:**
If firewall issue:
  ```bash
  sudo ufw allow 16591:16593/tcp
  ```

If bad peer:
  - Re-run init with good peer URL
  - Use deploy-follower-node prompt with new peer

---

### SYMPTOM: not_syncing (Has peers but blocks not advancing)

**Likely Causes:**
1. Database corruption
2. Old/incompatible snapshot
3. Configuration mismatch
4. Disk space full

**Resolution Steps:**

A. Check disk space
   ```bash
   df -h {work_dir}
   ```
   If <10% free → Disk full

B. Check database health
   - Look for "database" errors in logs
   - Check {work_dir}/dnn and /bvnn intact

C. Verify snapshot compatibility
   - Snapshots should be < 1 month old
   - Must match network (mainnet vs testnet)

D. Check configuration
   - Review accumulated.toml
   - Verify network settings match

**Fix:**
If disk full:
  - Free up space
  - Consider larger volume

If database issue:
  - May need to re-deploy with fresh snapshots
  - Use deploy-follower-node with recent snapshots

If config issue:
  - Restore from backup or re-init

---

### SYMPTOM: slow_sync (Syncing but very slow)

**Likely Causes:**
1. Limited peers (1-2 instead of 3+)
2. Slow storage (HDD vs SSD)
3. Network bandwidth limited
4. CPU/memory constrained

**Resolution Steps:**

A. Check resources
   ```bash
   htop  # Check CPU/memory
   iostat -x 1  # Check disk I/O
   ```

B. Verify peer count
   - Need 3+ peers for optimal sync
   - If <3, may need better peer URLs

C. Check network bandwidth
   - Syncing requires sustained download
   - Monitor with `iftop` or similar

**Fix:**
If resource constrained:
  - Upgrade to SSD storage
  - Increase memory allocation
  - Use less loaded system

If peer limited:
  - Add more peer URLs to config
  - Ensure ports not rate-limited

---

### SYMPTOM: crashed (Process died)

**Likely Causes:**
1. Out of memory
2. Database corruption
3. Bug/panic in code
4. Disk full

**Resolution Steps:**

A. Check crash logs
   - Review accumulated.log
   - Look for "panic" or "fatal"
   - Note exact error message

B. Check system resources
   ```bash
   dmesg | grep -i killed  # OOM killer?
   df -h  # Disk space?
   free -h  # Memory available?
   ```

C. Try restart
   - Use `accumulate_run_follower`
   - Monitor if crashes again

**Fix:**
If OOM:
  - Increase system memory
  - Add swap space
  - Reduce other processes

If database corruption:
  - Re-deploy with fresh snapshots

If persistent crash:
  - Report bug with logs
  - Try older/newer binary version

---

**General Recovery Procedure:**

1. Stop follower
2. Backup current state
3. Review all diagnostics above
4. Apply specific fix
5. Restart follower
6. Monitor for 10-15 minutes
7. If still failing → escalate or re-deploy

**When to Re-deploy:**

Consider fresh deployment if:
- Database corruption confirmed
- Snapshots > 1 month old
- Configuration completely broken
- Multiple fixes attempted without success

Use `deploy-follower-node` with fresh snapshots

**Prevention:**

- Monitor regularly with `monitor-follower-health`
- Keep snapshots recent (< 2 weeks)
- Ensure adequate resources
- Regular backups
- Update to latest stable version

**Related Prompts:**
- `monitor-follower-health` - Regular monitoring
- `deploy-follower-node` - Fresh deployment
- `backup-follower` - Create backup before major changes
```

---

## Prompt 4: setup-dev-wallet ⭐⭐

**Priority**: MEDIUM
**Combines**: 5+ tools
**Use Case**: Set up wallet for development/testing

### Specification

```go
{
    Name:        "setup-dev-wallet",
    Description: "Set up Accumulate wallet for development with testnet tokens",
    Arguments: []PromptArgument{
        {
            Name:        "network",
            Description: "Network to use: testnet or devnet (default: testnet)",
            Required:    false,
        },
        {
            Name:        "wallet_dir",
            Description: "Wallet directory path",
            Required:    false,
        },
        {
            Name:        "password",
            Description: "Wallet password (or use no_password for dev)",
            Required:    false,
        },
        {
            Name:        "no_password",
            Description: "Initialize without password for development",
            Required:    false,
        },
    },
}
```

### Template (Abbreviated)

```markdown
Setup development wallet for network: {network or testnet}

**Steps:**
1. wallet_init
2. wallet_vault_open
3. wallet_generate_key (2-3 keys)
4. wallet_set_network
5. accumulate_create_lite_account
6. accumulate_faucet (get testnet tokens)
7. wallet_get_status (verify)

[Full detailed steps with validation]
```

---

## Prompt 5: quick-node-status ⭐

**Priority**: MEDIUM
**Combines**: 2 tools
**Use Case**: Fast status check (<15 lines)

### Specification

```go
{
    Name:        "quick-node-status",
    Description: "Quick status check for follower node (concise output)",
    Arguments: []PromptArgument{
        {
            Name:        "work_dir",
            Description: "Follower working directory",
            Required:    false,
        },
    },
}
```

### Template (Abbreviated)

```markdown
Quick Node Status

Use: accumulate_follower_status + accumulate_node_info

**Output (concise):**
📊 Process: [RUNNING/STOPPED]
🔗 Peers: [count]
📦 Block: [height] (behind: [diff])
⏱️ Uptime: [duration]
✅ Status: [HEALTHY/WARNING/ERROR]

[Actions if not healthy]
```

---

## Implementation Priority

1. ✅ **deploy-follower-node** - Most complex, highest value
2. ✅ **monitor-follower-health** - High frequency use
3. ✅ **troubleshoot-follower-sync** - Critical for support
4. **setup-dev-wallet** - Developer onboarding
5. **quick-node-status** - Convenience

---

## Next Phase

**Phase 4**: Implement these 5 prompts in Go
- Create prompts.go
- Integrate with server
- Test with real follower deployment
