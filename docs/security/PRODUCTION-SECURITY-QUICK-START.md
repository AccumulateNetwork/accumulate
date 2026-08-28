# Production Security Quick Start Guide

**Target Audience:** Developers preparing for production launch
**Time to Read:** 5 minutes
**Full Plan:** See `PRODUCTION-SECURITY-PLAN.md`

---

## TL;DR - What Needs to Be Fixed

### 🔴 BLOCKING PRODUCTION (Must Fix Before Launch)

1. **Duplicate Vote Spam Attack**
   - **File:** `pkg/consensus/primary/vote_handler.go:72-87`
   - **Fix:** Move duplicate check before vote limit check
   - **Time:** 1 week (implementation + testing)
   - **Issue:** Byzantine validators can halt consensus

2. **Vote Verification Ordering**
   - **File:** `pkg/consensus/primary/vote_handler.go:26-37`
   - **Fix:** Check committee membership before signature verification
   - **Time:** 1 week (implementation + testing)
   - **Issue:** CPU exhaustion from non-validator spam

### 🟢 TEST INFRASTRUCTURE (No Action Required)

All test infrastructure security issues are **ACCEPTABLE** for development:
- ✅ Docker security issues - No Docker in production
- ✅ Dashboard security - No dashboard in production
- ✅ Monitoring security - Test-only tools

---

## Critical Fix #1: Duplicate Vote Spam

**Current code (VULNERABLE):**
```go
// Vote limit checked first
maxVotes := quorumCount * 2
if len(votes) >= maxVotes {
    return // REJECT
}

// Duplicate check second (TOO LATE)
for _, v := range votes {
    if bytes.Equal(v.Author, vote.Author) {
        return
    }
}
```

**Fixed code (SECURE):**
```go
// CHECK DUPLICATES FIRST
for _, v := range votes {
    if bytes.Equal(v.Author, vote.Author) {
        return errors.New("duplicate vote")
    }
}

// THEN check limit
maxVotes := quorumCount * 2
if len(votes) >= maxVotes {
    return errors.New("vote limit reached")
}
```

**Test:**
```bash
go test ./pkg/consensus/primary -run TestByzantineVoteSpam -v
```

---

## Critical Fix #2: Vote Verification Order

**Current code (VULNERABLE):**
```go
// Expensive signature verification first
if err := vote.Verify(); err != nil {
    return err
}

// Cheap committee check second
if !p.committee.ContainsValidator(vote.Author) {
    return errors.New("not in committee")
}
```

**Fixed code (SECURE):**
```go
// CHEAP check first (committee membership)
if !p.committee.ContainsValidator(vote.Author) {
    return errors.New("not in committee")
}

// EXPENSIVE check second (signature)
if err := vote.Verify(); err != nil {
    return err
}
```

**Test:**
```bash
go test ./pkg/consensus/primary -run TestNonValidatorVoteSpam -v
```

---

## Production Architecture Differences

### Test Deployment (Current)
```
┌─────────────────────────────────────┐
│   Single Server (Docker Compose)    │
│                                     │
│  ┌───────┐ ┌───────┐ ┌───────┐    │
│  │BVN1-1 │ │BVN1-2 │ │BVN1-3 │    │
│  └───────┘ └───────┘ └───────┘    │
│  ┌───────┐ ┌───────┐ ┌───────┐    │
│  │BVN2-1 │ │BVN2-2 │ │BVN2-3 │    │
│  └───────┘ └───────┘ └───────┘    │
│                                     │
└─────────────────────────────────────┘
```

### Production Deployment (Target)
```
┌──────────┐  ┌──────────┐  ┌──────────┐
│ AWS EC2  │  │  GCP VM  │  │Self-Host │
│          │  │          │  │          │
│┌────────┐│  │┌────────┐│  │┌────────┐│
││Validator││  ││Validator││  ││Validator││
││   #1   ││  ││   #2   ││  ││   #20  ││
│└────────┘│  │└────────┘│  │└────────┘│
└────┬─────┘  └────┬─────┘  └────┬─────┘
     │             │             │
     └─────────TLS P2P───────────┘
```

**Key Differences:**
- ✅ Separate physical/virtual servers
- ✅ 20+ validators (not 12)
- ✅ Independent operators
- ✅ TLS-encrypted communication
- ✅ HSM/KMS for key storage (not files)

---

## Week 1-2 Action Plan

### Day 1-2: Code Fixes
```bash
# Create feature branch
git checkout -b fix/consensus-security-prod

# Fix duplicate vote spam
vim pkg/consensus/primary/vote_handler.go
# (Apply fix from above)

# Fix vote verification order
vim pkg/consensus/primary/vote_handler.go
# (Apply fix from above)

# Commit
git add pkg/consensus/primary/vote_handler.go
git commit -m "Fix consensus security issues for production

- Fix duplicate vote spam attack (PROD-CONSENSUS-001)
- Reorder vote verification for DoS prevention (PROD-CONSENSUS-002)

Both issues are blocking production launch."
```

### Day 3-4: Unit Tests
```bash
# Add test for duplicate vote spam
vim pkg/consensus/primary/vote_handler_test.go

# Add test for vote verification order
vim pkg/consensus/primary/vote_handler_test.go

# Run tests
go test ./pkg/consensus/primary -v -race

# Commit tests
git add pkg/consensus/primary/vote_handler_test.go
git commit -m "Add security tests for consensus fixes"
```

### Day 5-7: Integration Testing
```bash
# Deploy 25-node testnet
cd test/docker
docker-compose -f docker-compose-25node.yml up -d

# Run Byzantine attack simulation
go run test/consensus/byzantine-attack-sim.go

# Verify consensus maintains liveness
# Expected: Blocks continue to be produced
# Expected: TPS degradation < 20%

# Load test under attack
go run test/load/attack-load-test.go

# Expected: 10K+ TPS maintained
# Expected: No consensus stalls
```

### Day 8-10: Security Review
```bash
# Code review (require 2+ approvals)
git push origin fix/consensus-security-prod
# Create PR, request reviews

# Security analysis
# - Manual code review by security lead
# - Static analysis (gosec, etc.)
# - Consider external audit if time permits

# Merge after approval
git checkout main
git merge fix/consensus-security-prod
```

---

## Production Deployment Timeline

### Week 1-2: Critical Fixes ← **YOU ARE HERE**
- Fix duplicate vote spam
- Fix vote verification ordering
- Test with 25-node network

### Week 3: Infrastructure Setup
- Provision 20+ validator servers
- Configure TLS for P2P
- Set up HSM/KMS for signing keys
- Configure firewalls

### Week 4: Monitoring & Docs
- Deploy Prometheus + Grafana
- Create operational runbooks
- Document key rotation procedures
- Set up NTP time sync

### Week 5: Launch
- Generate genesis block
- Create validator signing keys
- Launch production network
- 24-hour monitoring period

---

## Who Does What

**Core Developers:**
- Fix consensus vulnerabilities
- Write unit tests
- Code review

**QA Team:**
- Integration testing
- Byzantine attack simulation
- Load testing under attack

**DevOps:**
- Deploy 25-node testnet
- Provision production infrastructure
- Configure TLS, firewalls, monitoring

**Security Team:**
- Code review
- Security analysis
- External audit coordination

**SRE:**
- Monitoring setup
- Runbook creation
- Incident response planning

---

## Success Criteria

### Code Changes
- [ ] Duplicate vote spam fix implemented
- [ ] Vote verification reordering implemented
- [ ] Unit tests pass (including new security tests)
- [ ] Integration tests pass (25-node network)
- [ ] Code review approved by 2+ engineers

### Testing
- [ ] Byzantine attack simulation: Consensus maintains liveness
- [ ] Load test under attack: TPS > 10K sustained
- [ ] Fuzz testing: No crashes or panics
- [ ] 24-hour soak test: No degradation

### Production Readiness
- [ ] 20+ validator servers provisioned
- [ ] TLS configured and tested
- [ ] HSM/KMS deployed for 10+ validators
- [ ] Monitoring operational
- [ ] Runbooks complete
- [ ] Team trained on incident response

---

## Emergency Contacts

**If issues found during security review:**
- Security Lead: [Email, Phone]
- Engineering Lead: [Email, Phone]

**External security audit (if needed):**
- Trail of Bits: contact@trailofbits.com
- NCC Group: info@nccgroup.com

---

## Questions?

**"Can we launch without fixing these?"**
- NO - These are consensus-level vulnerabilities that could halt the network

**"Can we fix these after launch?"**
- NO - Exploited vulnerabilities cannot be fixed while under attack

**"How urgent is this?"**
- CRITICAL - Blocking production launch, should be top priority

**"What if we only have 10 validators instead of 20?"**
- Still need to fix - attack works with any number of validators >= 4

**"Can we use file-based keys instead of HSM?"**
- For testnet: Yes
- For production: Not recommended, but acceptable if properly encrypted

---

## Next Steps

1. **Read this document** (you just did ✓)
2. **Review full plan:** `PRODUCTION-SECURITY-PLAN.md`
3. **Create feature branch:** `git checkout -b fix/consensus-security-prod`
4. **Implement fixes:** Apply code changes from above
5. **Write tests:** Add security test cases
6. **Submit for review:** Create PR with detailed description
7. **Deploy testnet:** Test with 25-node network
8. **Launch production:** After all checks pass

---

**Last Updated:** March 24, 2026
**Owner:** Security Team
**Status:** Active - Awaiting Implementation
