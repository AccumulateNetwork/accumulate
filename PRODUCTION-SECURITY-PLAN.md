# Accumulate Production Security Plan

**Document Version:** 1.0
**Date:** March 24, 2026
**Status:** Draft for Review
**Target:** Production deployment with 20+ validators on distributed infrastructure

---

## Executive Summary

This document outlines the security requirements and remediation plan for deploying Accumulate to production. It distinguishes between **test infrastructure security issues** (acceptable for development) and **production consensus/protocol security issues** (must be fixed).

**Key Findings:**
- ✅ **Test infrastructure insecurities**: Acceptable (Docker, monitoring, dashboard)
- ⚠️ **2 Critical consensus issues**: Must fix before production
- ⚠️ **3 High-priority operational issues**: Should fix before launch
- 📋 **Production deployment architecture**: Requires separate planning

**Timeline:**
- **Blocking (Week 1-2)**: Fix consensus vulnerabilities
- **Pre-Launch (Week 3-4)**: Operational security, key management
- **Post-Launch (Month 2-3)**: Enhanced monitoring, key rotation

---

## Table of Contents

1. [Test vs Production Security](#test-vs-production-security)
2. [Critical Production Vulnerabilities](#critical-production-vulnerabilities)
3. [Production Deployment Architecture](#production-deployment-architecture)
4. [Remediation Roadmap](#remediation-roadmap)
5. [Operational Security Requirements](#operational-security-requirements)
6. [Production Readiness Checklist](#production-readiness-checklist)
7. [Post-Launch Security Enhancements](#post-launch-security-enhancements)

---

## Test vs Production Security

### Test Infrastructure (Acceptable Insecurities)

The following issues were identified in **test/development infrastructure only** and are **NOT applicable to production**:

| Issue | Severity | Why Acceptable for Test | Production Status |
|-------|----------|------------------------|-------------------|
| Docker command injection | Critical | Hardcoded values, isolated test env | ✅ N/A - No Docker in prod |
| Dashboard lacks authentication | High | Localhost only, dev environment | ✅ N/A - No dashboard in prod |
| CORS wildcard policy | High | Test UI convenience | ✅ N/A - No web UI in prod |
| XSS in monitoring dashboard | Medium | Not exposed to untrusted users | ✅ N/A - Test-only tool |
| CSV injection in exports | Medium | Internal test data only | ✅ N/A - Test-only monitoring |
| Unencrypted HTTP metrics | Medium | Localhost communication | ✅ N/A - Test-only API |
| Missing rate limiting | Low | Controlled test environment | ✅ N/A - Test-only |

**Conclusion**: All test infrastructure issues are **acceptable as-is**. No action required for production.

---

### Production Protocol Security (Requires Action)

The following issues affect the **consensus protocol and validator software** running in production:

| Issue | Severity | Production Impact | Action Required |
|-------|----------|------------------|-----------------|
| Duplicate vote spam attack | **CRITICAL** | Consensus halt | **BLOCKING** - Must fix |
| Vote verification ordering | **HIGH** | CPU DoS vulnerability | **BLOCKING** - Must fix |
| No key rotation mechanism | **HIGH** | Compromised keys persist | **PRE-LAUNCH** - Plan required |
| Timestamp replay protection | **MEDIUM** | Clock skew issues | **PRE-LAUNCH** - Document requirements |
| LRU eviction locking | **MEDIUM** | Performance degradation | **POST-LAUNCH** - Optimize |

---

## Critical Production Vulnerabilities

### 1. BLOCKING: Duplicate Vote Spam Attack

**Vulnerability ID:** PROD-CONSENSUS-001
**Severity:** CRITICAL
**Status:** 🔴 BLOCKING PRODUCTION LAUNCH

#### Problem Statement

Byzantine validators within the fault tolerance threshold (f+1 out of n validators) can prevent consensus by spamming duplicate votes from the same validator key.

#### Technical Details

**File:** `pkg/consensus/primary/vote_handler.go:72-87`

**Current vulnerable code:**
```go
// Vote limit checked first
maxVotes := quorumCount * VotesPerHeaderMultiplier // 2x quorum
votes := p.pendingVotes[vote.HeaderDigest]
if len(votes) >= maxVotes {
    return // Reject - buffer full
}

// Duplicate check happens second (TOO LATE)
for _, v := range votes {
    if bytes.Equal(v.Author, vote.Author) {
        return // Already have vote from this author
    }
}
votes = append(votes, vote)
```

**Attack Scenario with 25 Validators:**
```
Network: 25 validators
Quorum: 17 validators (2f+1 where f=8)
Max votes: 34 (2 × 17)

Attacker controls: 9 validators (within Byzantine tolerance)
Attack: Each validator sends 4 duplicate votes
Total: 9 × 4 = 36 votes

Result: Buffer full (36 > 34), honest votes rejected, consensus stalled
```

#### Impact Analysis

- **Consensus Liveness:** Compromised - network can halt indefinitely
- **Byzantine Tolerance:** Violated - attack works within f+1 threshold
- **Economic Impact:** Network downtime, transaction processing stops
- **Reputation Risk:** Critical security vulnerability if exploited

#### Required Fix

**Code Change:**
```go
// CHECK DUPLICATES FIRST
for _, v := range votes {
    if bytes.Equal(v.Author, vote.Author) {
        return // Duplicate from same author - reject immediately
    }
}

// THEN check vote limit (now counting unique votes only)
maxVotes := quorumCount * VotesPerHeaderMultiplier
if len(votes) >= maxVotes {
    return // Too many unique votes
}

votes = append(votes, vote)
p.pendingVotes[vote.HeaderDigest] = votes
```

**Alternative Fix (More Efficient):**
```go
// Use map for O(1) duplicate detection
type voteSet struct {
    votes      []*types.Vote
    seenAuthors map[string]bool // author hash -> seen
}

func (p *Primary) ProcessVote(vote *types.Vote) error {
    authorKey := string(vote.Author)

    voteSet := p.pendingVotes[vote.HeaderDigest]
    if voteSet == nil {
        voteSet = &voteSet{
            votes:       make([]*types.Vote, 0),
            seenAuthors: make(map[string]bool),
        }
    }

    // O(1) duplicate check
    if voteSet.seenAuthors[authorKey] {
        return errors.New("duplicate vote from author")
    }

    // Check unique author limit
    maxVotes := p.committee.Quorum() * VotesPerHeaderMultiplier
    if len(voteSet.votes) >= maxVotes {
        return errors.New("vote limit reached")
    }

    voteSet.votes = append(voteSet.votes, vote)
    voteSet.seenAuthors[authorKey] = true
    p.pendingVotes[vote.HeaderDigest] = voteSet
}
```

#### Testing Requirements

**Unit Tests:**
```go
// Test case: Duplicate votes from same validator
func TestRejectDuplicateVotes(t *testing.T) {
    // Setup: 7 validators, quorum=5, max=10
    // Send 3 votes from validator A
    // Expect: First accepted, second and third rejected
}

// Test case: Vote limit with unique authors
func TestVoteLimitWithUniqueAuthors(t *testing.T) {
    // Setup: 7 validators, quorum=5, max=10
    // Send 10 votes from different validators
    // Send 11th vote
    // Expect: 11th vote rejected (limit reached)
}

// Test case: Byzantine attack scenario
func TestByzantineVoteSpam(t *testing.T) {
    // Setup: 25 validators, f=8, quorum=17, max=34
    // Attacker: 9 validators send 4 votes each
    // Expect: Only 9 votes accepted (one per validator)
    // Expect: Honest validators can still reach quorum
}
```

**Integration Tests:**
```go
// Test case: Full consensus with Byzantine validators
func TestConsensusWithByzantineVoteSpammers(t *testing.T) {
    // Setup: 25-node network
    // Inject: 9 malicious validators spamming duplicates
    // Verify: Consensus still reaches finality
    // Verify: Blocks continue to be produced
}
```

#### Timeline

- **Week 1**: Implement fix and unit tests
- **Week 1**: Code review and security analysis
- **Week 2**: Integration testing with 25-node testnet
- **Week 2**: Fuzz testing and edge cases
- **Acceptance**: Pass all tests, independent security review

---

### 2. BLOCKING: Vote Verification CPU Exhaustion

**Vulnerability ID:** PROD-CONSENSUS-002
**Severity:** HIGH
**Status:** 🔴 BLOCKING PRODUCTION LAUNCH

#### Problem Statement

Non-committee nodes can spam votes that trigger expensive cryptographic signature verification (ed25519, ~50-100µs each) before the cheap committee membership check occurs.

#### Technical Details

**File:** `pkg/consensus/primary/vote_handler.go:26-37`

**Current vulnerable code:**
```go
// EXPENSIVE: Signature verification first (50-100µs)
if err := vote.Verify(); err != nil {
    return
}

// CHEAP: Committee check second (O(n) lookup, <1µs)
p.committeeMu.RLock()
inCommittee := p.committee.ContainsValidator(vote.Author)
p.committeeMu.RUnlock()

if !inCommittee {
    return // Not in committee - wasted CPU on verification
}
```

**Attack Scenario:**
```
Attacker: Controls 100 non-validator nodes
Attack rate: 10,000 votes/second per validator
Signature verification: 100µs per vote
CPU cost: 10,000 × 0.1ms = 1 full CPU core per validator

With 20 validators under attack:
Total CPU waste: 20 cores doing useless signature verification
```

#### Impact Analysis

- **CPU Exhaustion:** Validators can be DoS'd by non-committee nodes
- **Network Degradation:** Legitimate consensus messages delayed
- **Gossip Amplification:** Attack traffic consumes network bandwidth
- **Cost:** Increased cloud compute costs under sustained attack

#### Required Fix

**Code Change:**
```go
// CHEAP CHECK FIRST: Committee membership (~1µs)
p.committeeMu.RLock()
inCommittee := p.committee.ContainsValidator(vote.Author)
p.committeeMu.RUnlock()

if !inCommittee {
    // Reject immediately - don't waste CPU on signature
    return errors.New("vote from non-committee member")
}

// EXPENSIVE CHECK SECOND: Signature verification (50-100µs)
if err := vote.Verify(); err != nil {
    return err
}
```

**Additional Hardening:**

1. **Rate Limiting per Peer:**
```go
// In gossip layer
type peerRateLimit struct {
    mu            sync.Mutex
    votesPerPeer  map[string]*rate.Limiter
}

func (p *peerRateLimit) allowVote(peerID string) bool {
    p.mu.Lock()
    defer p.mu.Unlock()

    limiter := p.votesPerPeer[peerID]
    if limiter == nil {
        // 100 votes/second per peer
        limiter = rate.NewLimiter(100, 200)
        p.votesPerPeer[peerID] = limiter
    }

    return limiter.Allow()
}
```

2. **Signature Verification Cache:**
```go
// Cache recent signature verifications
type sigCache struct {
    cache *lru.Cache // sig hash -> validity
}

func (p *Primary) verifyWithCache(vote *types.Vote) error {
    sigHash := sha256.Sum256(vote.Signature)

    if valid, ok := p.sigCache.Get(sigHash); ok {
        if valid.(bool) {
            return nil
        }
        return errors.New("cached invalid signature")
    }

    err := vote.Verify()
    p.sigCache.Add(sigHash, err == nil)
    return err
}
```

#### Testing Requirements

**Load Test:**
```bash
# Simulate non-committee vote spam
# 100 non-validator nodes send 10,000 votes/sec
# Monitor CPU usage on validators
# Expect: CPU usage < 10% for invalid votes
```

**Benchmark:**
```go
func BenchmarkVoteValidation(b *testing.B) {
    // Benchmark: Committee check only (should be <1µs)
    // Benchmark: Signature verification only (should be 50-100µs)
    // Benchmark: Full validation with reordering
}
```

#### Timeline

- **Week 1**: Implement check reordering
- **Week 1**: Add rate limiting in gossip layer
- **Week 2**: Implement signature cache
- **Week 2**: Load testing with attack simulation
- **Acceptance**: CPU usage under attack < 20%

---

## Production Deployment Architecture

### Reference Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Production Network                        │
│                   (20+ Validators)                           │
└─────────────────────────────────────────────────────────────┘

┌──────────────────┐      ┌──────────────────┐      ┌──────────────────┐
│   Validator 1    │      │   Validator 2    │      │   Validator 20   │
│                  │      │                  │      │                  │
│  ┌────────────┐  │      │  ┌────────────┐  │      │  ┌────────────┐  │
│  │ Accumulated│  │      │  │ Accumulated│  │      │  │ Accumulated│  │
│  │  Process   │  │      │  │  Process   │  │      │  │  Process   │  │
│  └────────────┘  │      │  └────────────┘  │      │  └────────────┘  │
│        │         │      │        │         │      │        │         │
│  ┌────────────┐  │      │  ┌────────────┐  │      │  ┌────────────┐  │
│  │   BadgerDB │  │      │  │   BadgerDB │  │      │  │   BadgerDB │  │
│  └────────────┘  │      │  └────────────┘  │      │  └────────────┘  │
│        │         │      │        │         │      │        │         │
│  ┌────────────┐  │      │  ┌────────────┐  │      │  ┌────────────┐  │
│  │ Signing Key│  │      │  │ Signing Key│  │      │  │ Signing Key│  │
│  │    (HSM)   │  │      │  │   (File)   │  │      │  │   (KMS)    │  │
│  └────────────┘  │      │  └────────────┘  │      │  └────────────┘  │
│                  │      │                  │      │                  │
│  AWS EC2         │      │  Self-Hosted     │      │  Google Cloud    │
│  i3.2xlarge      │      │  Physical Server │      │  n2-standard-8   │
└──────────────────┘      └──────────────────┘      └──────────────────┘
         │                         │                          │
         └─────────────────────────┴──────────────────────────┘
                                   │
                         TLS Encrypted P2P
                         (libp2p with TLS 1.3)
```

### Infrastructure Requirements

#### Per-Validator Server Specifications

**Minimum Requirements:**
- **CPU:** 4 cores (8 recommended for headroom)
- **RAM:** 16 GB (32 GB recommended)
- **Storage:** 500 GB SSD (NVMe preferred)
- **Network:** 1 Gbps symmetric (10 Gbps for high-traffic BVNs)
- **OS:** Ubuntu 22.04 LTS (or equivalent)

**Recommended Cloud Instances:**
- **AWS:** i3.2xlarge (8 vCPU, 61 GB RAM, NVMe SSD)
- **Google Cloud:** n2-standard-8 + persistent SSD
- **Azure:** Standard_D8s_v3 + Premium SSD
- **Self-Hosted:** Dedicated server with RAID SSD

#### Network Configuration

**Required Ports:**
```
Inbound:
- 16593/tcp: P2P gossip (libp2p)
- 26660/tcp: JSON-RPC API (optional, can be firewalled)

Outbound:
- 16593/tcp: P2P to other validators
- 443/tcp: NTP time sync (time.google.com)
- 123/udp: NTP fallback
```

**Firewall Rules:**
```bash
# Allow P2P from other validators only
iptables -A INPUT -p tcp --dport 16593 -s <validator-ip-range> -j ACCEPT
iptables -A INPUT -p tcp --dport 16593 -j DROP

# Allow API from specific IPs only (or block entirely)
iptables -A INPUT -p tcp --dport 26660 -s <admin-ip> -j ACCEPT
iptables -A INPUT -p tcp --dport 26660 -j DROP

# Allow NTP for clock sync
iptables -A OUTPUT -p udp --dport 123 -j ACCEPT
```

#### TLS Configuration

**Validator-to-Validator Communication:**
```yaml
# libp2p TLS configuration
tls:
  min_version: TLS1.3
  cipher_suites:
    - TLS_AES_256_GCM_SHA384
    - TLS_CHACHA20_POLY1305_SHA256

  # Certificate validation
  verify_peer: true
  verify_chain: true

  # Certificate pinning (optional, for known validators)
  pinned_certs:
    - fingerprint: "SHA256:..."
```

**Certificate Management:**
```bash
# Generate validator TLS certificate
openssl req -x509 -newkey rsa:4096 -sha256 -days 365 \
  -nodes -keyout validator.key -out validator.crt \
  -subj "/CN=validator-1.accumulate.network"

# Or use Let's Encrypt for public validators
certbot certonly --standalone -d validator-1.accumulate.network
```

### Key Management

#### Validator Signing Keys

**Option 1: Hardware Security Module (HSM) - RECOMMENDED**

```yaml
# AWS CloudHSM configuration
key_management:
  type: hsm
  provider: aws-cloudhsm
  cluster_id: cluster-abc123

  # Key backup to separate HSM cluster
  backup:
    enabled: true
    backup_cluster: cluster-xyz789
```

**Benefits:**
- Keys never leave hardware
- FIPS 140-2 Level 3 certified
- Automatic key backup
- Audit logging

**Drawbacks:**
- Cost: ~$1-2/hour per HSM
- Complexity: Requires HSM configuration

---

**Option 2: Cloud KMS - RECOMMENDED for Cloud Deployments**

```yaml
# Google Cloud KMS configuration
key_management:
  type: kms
  provider: gcp-kms
  project: accumulate-prod
  location: us-central1
  keyring: validators
  key: validator-1-signing-key

  # Automatic rotation
  rotation:
    enabled: true
    interval: 90d  # Rotate every 90 days
```

**Benefits:**
- Managed service (no HSM maintenance)
- Automatic key rotation
- Audit logging built-in
- Cost-effective (~$0.06/10k operations)

**Drawbacks:**
- Requires cloud provider trust
- Network latency for signing operations

---

**Option 3: Encrypted File (Acceptable for Testnet)**

```yaml
# File-based key storage (NOT recommended for production)
key_management:
  type: file
  path: /secure/keys/validator.key

  # Must be encrypted at rest
  encryption:
    enabled: true
    algorithm: AES-256-GCM
    passphrase_source: environment  # Never hardcoded
```

**Security Requirements:**
```bash
# Strict file permissions
chmod 400 /secure/keys/validator.key
chown validator:validator /secure/keys/validator.key

# Encrypted filesystem
cryptsetup luksFormat /dev/sdb
mount /dev/mapper/secure /secure

# Key derivation from environment
export VALIDATOR_KEY_PASSPHRASE="$(vault kv get -field=passphrase secret/validator-1)"
```

**⚠️ WARNING:** File-based keys are vulnerable to:
- Server compromise (attacker copies encrypted key)
- Memory dumps (passphrase in RAM)
- Insider threats (admin access)

**Only acceptable for:**
- Testnet deployments
- Development environments
- Low-value networks

---

#### Key Rotation Strategy

**Rotation Schedule:**
```
Validator Signing Keys:
- Recommended: Every 90 days
- Minimum: Every 180 days
- Emergency: Immediate on suspected compromise

TLS Certificates:
- Recommended: Every 365 days (Let's Encrypt: 90 days auto-renewal)
- Minimum: Before expiration

API Keys (if applicable):
- Recommended: Every 30 days
- Minimum: Every 90 days
```

**Rotation Procedure:**
```
1. Generate new key in HSM/KMS
2. Submit key update transaction to network
3. Wait for transaction finality (2-3 blocks)
4. Update local validator configuration
5. Restart validator with new key
6. Verify signing with new key
7. Archive old key (DO NOT DELETE for 30 days)
8. Monitor for any issues
9. After 30 days: Revoke old key
```

**Automated Rotation (Example):**
```bash
#!/bin/bash
# rotate-validator-key.sh

# Generate new key
NEW_KEY=$(gcloud kms keys create "validator-1-$(date +%Y%m%d)" \
  --keyring=validators \
  --location=us-central1 \
  --purpose=asymmetric-signing \
  --default-algorithm=ec-sign-p256-sha256)

# Submit key rotation transaction
accumulated tx submit \
  --type=UpdateKey \
  --key-id=validator-1 \
  --new-key="$NEW_KEY" \
  --old-key-signature="$(sign-with-old-key "$NEW_KEY")"

# Wait for finality
sleep 30

# Update validator config
sed -i "s/signing-key: .*/signing-key: $NEW_KEY/" /etc/accumulated/config.yml

# Restart validator
systemctl restart accumulated

# Verify
accumulated query validator validator-1 --field=public-key
```

---

### Monitoring and Observability

#### Required Metrics (Per Validator)

**System Metrics:**
```
- CPU usage (%, per core)
- Memory usage (GB, %)
- Disk usage (GB, %)
- Disk I/O (IOPS, MB/s)
- Network I/O (packets/s, MB/s)
- Network latency to other validators (ms)
```

**Consensus Metrics:**
```
- Blocks proposed (count, rate)
- Votes cast (count, rate)
- Votes received (count, rate)
- Certificates created (count, rate)
- Round number (current)
- Epoch number (current)
- Pending batches (count)
- Batch queue depth (count)
```

**Database Metrics:**
```
- Database size (GB)
- Write rate (writes/s, MB/s)
- Read rate (reads/s, MB/s)
- Compaction time (seconds)
- LSM tree levels (count)
```

**Application Metrics:**
```
- Transactions processed (count, rate)
- Transaction success rate (%)
- Transaction latency (p50, p95, p99)
- API request rate (req/s)
- API error rate (%)
```

#### Monitoring Stack

**Recommended: Prometheus + Grafana**

```yaml
# prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'accumulate-validators'
    static_configs:
      - targets:
        - validator-1.internal:9090
        - validator-2.internal:9090
        # ... all validators

    metric_relabel_configs:
      # Drop high-cardinality metrics
      - source_labels: [__name__]
        regex: 'http_request_duration_.*'
        action: drop
```

**Alert Rules:**
```yaml
# alerts.yml
groups:
  - name: consensus
    interval: 30s
    rules:
      - alert: ValidatorNotProducingBlocks
        expr: rate(blocks_produced_total[5m]) == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Validator {{ $labels.instance }} not producing blocks"

      - alert: HighVoteRejectionRate
        expr: rate(votes_rejected_total[5m]) / rate(votes_received_total[5m]) > 0.1
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High vote rejection rate on {{ $labels.instance }}"

      - alert: DatabaseGrowthExcessive
        expr: rate(database_size_bytes[1h]) > 1e9  # 1GB/hour
        for: 1h
        labels:
          severity: warning
        annotations:
          summary: "Excessive database growth on {{ $labels.instance }}"
```

**Grafana Dashboards:**
```
Dashboard 1: Network Overview
- Total validators online
- Network TPS
- Block production rate
- Consensus round progression

Dashboard 2: Per-Validator Health
- CPU, memory, disk usage
- Vote participation rate
- Block proposal success rate
- Database size and growth

Dashboard 3: Consensus Deep Dive
- Vote message flow
- Certificate creation timeline
- Round advancement latency
- Batch queue metrics
```

---

## Remediation Roadmap

### Phase 1: Blocking Issues (Week 1-2)

**Goal:** Fix critical consensus vulnerabilities blocking production launch

#### Week 1: Implementation

| Task | Owner | Deliverable | Acceptance Criteria |
|------|-------|-------------|---------------------|
| Fix duplicate vote spam attack | Core Dev | PR with fix + tests | All tests pass, code review approved |
| Reorder vote verification | Core Dev | PR with fix + benchmarks | CPU usage < 10% under spam attack |
| Add vote spam integration test | QA | Test suite | 25-node network survives attack |
| Security code review | Security Lead | Review report | No additional critical issues found |

#### Week 2: Testing and Validation

| Task | Owner | Deliverable | Acceptance Criteria |
|------|-------|-------------|---------------------|
| 25-node testnet deployment | DevOps | Running testnet | All validators healthy |
| Byzantine attack simulation | QA | Attack test results | Consensus maintains liveness |
| Load testing under attack | QA | Performance report | TPS degradation < 20% |
| Fuzz testing vote handler | QA | Fuzz test results | No crashes or panics |

**Milestone:** Critical vulnerabilities patched and verified

---

### Phase 2: Pre-Launch (Week 3-4)

**Goal:** Operational security, key management, deployment preparation

#### Week 3: Infrastructure Setup

| Task | Owner | Deliverable | Acceptance Criteria |
|------|-------|-------------|---------------------|
| Deploy 20 validator nodes | DevOps | Running validators | All nodes syncing |
| Configure TLS for P2P | DevOps | TLS config | All connections encrypted |
| Set up HSM/KMS for 10+ validators | DevOps | Key management | Keys inaccessible to operators |
| Configure firewalls | SecOps | Firewall rules | Only required ports open |

#### Week 4: Monitoring and Documentation

| Task | Owner | Deliverable | Acceptance Criteria |
|------|-------|-------------|---------------------|
| Deploy Prometheus + Grafana | DevOps | Monitoring stack | All metrics collecting |
| Create runbooks | SRE | Runbook documentation | Cover all incidents |
| Document key rotation procedure | SecOps | Key rotation guide | Tested on testnet |
| NTP time sync setup | DevOps | NTP config | Clock drift < 100ms |
| Incident response plan | SecOps | IR playbook | Team trained |

**Milestone:** Production infrastructure ready, monitoring operational

---

### Phase 3: Launch (Week 5)

**Goal:** Production network launch with security validation

| Task | Owner | Deliverable | Acceptance Criteria |
|------|-------|-------------|---------------------|
| Genesis block creation | Core Dev | Genesis file | Verified by all validators |
| Validator key generation | SecOps | 20+ validator keys | All in HSM/KMS |
| Production launch | All | Live network | Consensus achieved |
| 24-hour monitoring | SRE | Health report | No critical alerts |
| Security audit | External | Audit report | No critical findings |

**Milestone:** Production network live and stable

---

### Phase 4: Post-Launch (Month 2-3)

**Goal:** Enhanced security, optimization, key rotation implementation

#### Month 2: Optimization

| Task | Owner | Deliverable | Acceptance Criteria |
|------|-------|-------------|---------------------|
| Optimize LRU eviction locking | Core Dev | Performance improvement | Lock contention < 5% |
| Implement signature verification cache | Core Dev | Cache implementation | Cache hit rate > 90% |
| Add rate limiting per peer | Core Dev | Rate limiter | CPU under spam < 20% |
| Performance testing | QA | Load test report | 15K+ TPS sustained |

#### Month 3: Advanced Features

| Task | Owner | Deliverable | Acceptance Criteria |
|------|-------|-------------|---------------------|
| Implement automated key rotation | SecOps | Key rotation system | Successful rotation on testnet |
| Add timestamp bounds checking | Core Dev | Timestamp validation | Clock skew < 5 minutes tolerated |
| Enhanced monitoring dashboards | SRE | Grafana dashboards | All metrics visualized |
| Disaster recovery testing | SRE | DR test report | Recovery time < 1 hour |

**Milestone:** Production network optimized and hardened

---

## Operational Security Requirements

### Access Control

#### Validator Server Access

**SSH Access:**
```bash
# Require SSH keys only (no passwords)
PasswordAuthentication no
PubkeyAuthentication yes

# Limit to specific users
AllowUsers validator-admin

# Disable root login
PermitRootLogin no

# Two-factor authentication (optional but recommended)
AuthenticationMethods publickey,keyboard-interactive:pam
```

**Privilege Escalation:**
```bash
# Require password for sudo
Defaults timestamp_timeout=5

# Log all sudo commands
Defaults log_input, log_output
Defaults logfile="/var/log/sudo.log"

# Restrict sudo to specific commands
validator-admin ALL=(ALL) /bin/systemctl restart accumulated
validator-admin ALL=(ALL) /bin/journalctl -u accumulated
```

**Bastion Host (Recommended):**
```
Internet → Bastion Host → Validators
          (Public IP)    (Private IPs only)

Bastion security:
- Minimal software (SSH only)
- Aggressive fail2ban rules
- SSH session recording
- MFA required
```

---

### Secrets Management

**DO NOT:**
- ❌ Hardcode secrets in code
- ❌ Store secrets in environment variables (visible in `ps`)
- ❌ Commit secrets to git
- ❌ Store secrets in plain text config files
- ❌ Share secrets via email/Slack

**DO:**
- ✅ Use secrets management system (HashiCorp Vault, AWS Secrets Manager)
- ✅ Rotate secrets regularly
- ✅ Encrypt secrets at rest
- ✅ Audit secret access
- ✅ Use short-lived credentials where possible

**Example: HashiCorp Vault Integration**

```bash
# Store validator key passphrase
vault kv put secret/validator-1 \
  passphrase="$(openssl rand -base64 32)"

# Retrieve in validator startup script
export VALIDATOR_KEY_PASSPHRASE="$(vault kv get -field=passphrase secret/validator-1)"

accumulated run --key-passphrase-env=VALIDATOR_KEY_PASSPHRASE
```

---

### Incident Response

#### Incident Classification

| Severity | Definition | Response Time | Examples |
|----------|-----------|---------------|----------|
| **P0 - Critical** | Network down, consensus halted | < 15 minutes | Byzantine attack, consensus bug |
| **P1 - High** | Validator offline, degraded performance | < 1 hour | Server outage, disk full |
| **P2 - Medium** | Non-critical issues, monitoring alerts | < 4 hours | High CPU, slow queries |
| **P3 - Low** | Minor issues, no immediate impact | < 24 hours | Log warnings, minor config |

#### Incident Response Procedures

**P0: Consensus Halted**

```
1. ALERT: Page on-call team immediately
2. ASSESS: Identify cause (Byzantine attack, bug, network partition)
3. MITIGATE:
   - If attack: Block malicious peers
   - If bug: Emergency patch or rollback
   - If partition: Restore network connectivity
4. RECOVER: Restart affected validators
5. VERIFY: Consensus resumed, blocks producing
6. POST-MORTEM: Root cause analysis, prevention plan
```

**P1: Validator Offline**

```
1. ALERT: Notify on-call engineer
2. DIAGNOSE:
   - Check server health (CPU, memory, disk)
   - Check network connectivity
   - Check process status (systemctl status accumulated)
   - Review logs (journalctl -u accumulated -n 1000)
3. RECOVER:
   - Restart service if crashed
   - Provision new server if hardware failure
   - Restore from backup if database corruption
4. VERIFY: Validator rejoins network, syncs to latest block
5. DOCUMENT: Incident in log, update runbook if needed
```

**Communication Plan:**

```
Stakeholders:
- Internal: Engineering team, management
- External: Validator operators, community (if public network)

Channels:
- Critical: PagerDuty, phone calls
- High: Slack #incidents channel
- Public: Status page (status.accumulate.network)

Timing:
- Initial alert: Within 5 minutes of detection
- Hourly updates: Until resolved
- Post-mortem: Within 48 hours
```

---

### Security Auditing

#### Code Audits

**Schedule:**
- **Pre-launch:** Full security audit by external firm
- **Major releases:** Audit of changed code
- **Annual:** Comprehensive re-audit

**Scope:**
- Consensus protocol implementation
- Cryptographic primitives
- Network protocol
- Key management
- Access control

**Recommended Auditors:**
- Trail of Bits
- NCC Group
- Kudelski Security
- Least Authority

---

#### Operational Audits

**Quarterly Reviews:**
```
Q1: Access control audit
- Review SSH keys
- Audit sudo logs
- Verify MFA enabled

Q2: Secrets management audit
- Rotate all credentials
- Audit Vault access logs
- Verify encryption at rest

Q3: Network security audit
- Penetration testing
- Firewall rule review
- TLS configuration audit

Q4: Incident response drill
- Simulate P0 incident
- Test communication plan
- Update runbooks
```

---

### Compliance (If Applicable)

**Data Privacy:**
- GDPR compliance (if EU validators)
- Data retention policies
- Right to erasure (challenging for blockchain)

**Financial Regulations:**
- SOC 2 Type II (if handling customer funds)
- ISO 27001 (information security management)
- PCI DSS (if processing card transactions)

---

## Production Readiness Checklist

### Critical (Blocking Launch)

- [ ] **PROD-CONSENSUS-001 Fixed:** Duplicate vote spam attack patched
- [ ] **PROD-CONSENSUS-002 Fixed:** Vote verification ordering optimized
- [ ] **Security Audit:** External audit completed, no critical findings
- [ ] **Testing:** 25-node testnet with Byzantine attack simulation passed
- [ ] **Code Review:** All consensus changes reviewed by 2+ engineers

### High Priority (Should Have Before Launch)

- [ ] **TLS Configured:** All validator-to-validator connections encrypted
- [ ] **Firewalls:** Only required ports open, admin access restricted
- [ ] **Key Management:** 10+ validators using HSM/KMS (not file-based keys)
- [ ] **Monitoring:** Prometheus + Grafana deployed, all metrics collecting
- [ ] **Alerts:** Critical alert rules configured and tested
- [ ] **NTP Sync:** All validators synchronized to < 100ms drift
- [ ] **Runbooks:** Incident response procedures documented and tested
- [ ] **Backups:** Database backup strategy implemented and tested

### Medium Priority (Can Launch Without, Fix Soon After)

- [ ] **Rate Limiting:** Per-peer vote rate limiting implemented
- [ ] **Signature Cache:** Signature verification caching for performance
- [ ] **Key Rotation:** Automated key rotation system (can be manual initially)
- [ ] **Disaster Recovery:** Full DR plan tested and documented
- [ ] **Performance Tuning:** LRU eviction locking optimized
- [ ] **Timestamp Bounds:** Maximum future timestamp enforcement

### Nice to Have (Post-Launch)

- [ ] **Advanced Monitoring:** Custom Grafana dashboards for all metrics
- [ ] **Log Aggregation:** Centralized logging (ELK, Splunk, or similar)
- [ ] **Chaos Engineering:** Automated failure injection testing
- [ ] **Multi-region:** Geographic distribution of validators
- [ ] **HSM for All:** 100% of validators using HSM (currently 50%+ acceptable)

---

## Post-Launch Security Enhancements

### Month 1-3: Immediate Improvements

**Week 1-2:**
- Monitor production for anomalies
- Tune alert thresholds based on actual behavior
- Address any discovered issues immediately

**Week 3-4:**
- Implement signature verification cache (if not already done)
- Add per-peer rate limiting (if not already done)
- Optimize LRU eviction locking

**Month 2:**
- Implement automated key rotation system
- Add timestamp bounds checking
- Conduct first quarterly security audit

**Month 3:**
- Chaos engineering: Inject failures, test resilience
- Performance optimization: Target 20K+ TPS
- Disaster recovery drill

---

### Month 4-6: Advanced Security

**Month 4:**
- Multi-region deployment (if applicable)
- Geographic distribution of validators
- Cross-region failover testing

**Month 5:**
- Implement advanced monitoring (anomaly detection)
- Machine learning for attack detection
- Automated response to common incidents

**Month 6:**
- SOC 2 Type II preparation (if applicable)
- ISO 27001 preparation (if applicable)
- Second external security audit

---

### Year 1+: Continuous Improvement

**Quarterly:**
- Security audits (code and operational)
- Penetration testing
- Incident response drills
- Compliance reviews

**Annually:**
- Full external security audit
- Cryptographic review (new attacks, deprecate weak crypto)
- Disaster recovery full-scale test
- Update security policies and procedures

**Ongoing:**
- Monitor CVEs for dependencies
- Update libraries and dependencies
- Review access control quarterly
- Rotate credentials on schedule
- Stay informed on blockchain security research

---

## Appendix A: Security Contacts

**Internal:**
- Security Lead: [Name, Email, Phone]
- On-Call Engineer: [PagerDuty, Phone]
- Incident Commander: [Name, Email, Phone]

**External:**
- Security Audit Firm: [Company, Contact, Phone]
- Legal Counsel: [Firm, Attorney, Phone]
- Insurance Provider: [Company, Policy #, Phone]

---

## Appendix B: References

**Security Standards:**
- NIST Cybersecurity Framework
- OWASP Top 10
- CIS Controls
- ISO 27001/27002

**Blockchain Security:**
- "Security Analysis of Proof-of-Stake Protocols" (Garay et al.)
- "The Bitcoin Backbone Protocol" (Garay et al.)
- Trail of Bits: Building Secure Smart Contracts

**Accumulate Documentation:**
- Protocol Specification: [Link]
- Consensus Documentation: [Link]
- Operator Manual: [Link]

---

## Document Control

**Version History:**

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-03-24 | Security Team | Initial production security plan |

**Approval:**

- [ ] Security Lead: _________________ Date: _______
- [ ] Engineering Lead: ______________ Date: _______
- [ ] Operations Lead: _______________ Date: _______
- [ ] Executive Sponsor: _____________ Date: _______

**Review Schedule:**
- **Next Review:** 2026-06-24 (3 months)
- **Frequency:** Quarterly or after major incidents

---

**END OF DOCUMENT**
