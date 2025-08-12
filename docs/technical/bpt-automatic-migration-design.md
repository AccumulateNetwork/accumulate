# Automatic BPT Migration Design

## Executive Summary

Deploy an update that automatically migrates accounts from state-based to chain-based storage whenever they are modified. Use "touch" transactions to proactively migrate high-priority accounts.

## Core Principle

**Every account modification triggers automatic migration** - no user choice, no complexity, just seamless improvement.

## Implementation

### 1. Simple Migration Wrapper

```go
// internal/database/account.go
func (a *Account) Directory() DirectoryInterface {
    // Check if already migrated
    if a.HasChainDirectory() {
        return a.chainDirectory
    }
    
    // Return auto-migrating wrapper
    return &AutoMigratingDirectory{
        account: a,
        legacy:  a.legacyDirectory,
    }
}

type AutoMigratingDirectory struct {
    account  *Account
    legacy   values.Set[*url.URL]
    migrated bool
}

// ANY operation triggers migration
func (d *AutoMigratingDirectory) Add(urls ...*url.URL) error {
    if !d.migrated {
        d.migrateNow()
    }
    return d.account.chainDirectory.Add(urls...)
}

func (d *AutoMigratingDirectory) Get() ([]*url.URL, error) {
    if !d.migrated {
        d.migrateNow()
    }
    return d.account.chainDirectory.Get()
}

func (d *AutoMigratingDirectory) migrateNow() error {
    // One-time migration
    oldData, _ := d.legacy.Get()
    
    // Create chain
    d.account.chainDirectory = NewChainDirectory()
    
    // Migrate data
    for _, url := range oldData {
        d.account.chainDirectory.Append(url)
    }
    
    // Clear old storage
    d.legacy.Clear()
    d.migrated = true
    
    return nil
}
```

### 2. BPT Hash Adaptation

```go
func (a *observedAccount) hashSecondaryState() (hash.Hasher, error) {
    var hasher hash.Hasher
    
    // Auto-detect storage type
    if a.HasChainDirectory() {
        // New: O(1) - just the chain anchor
        anchor := a.DirectoryChain().Anchor()
        hasher.AddHash(&anchor)
    } else {
        // Legacy: O(n) - hash all URLs
        for _, u := range a.LegacyDirectory().Get() {
            hasher.AddUrl(u)
        }
    }
    
    return hasher, nil
}
```

### 3. Touch Transaction for Proactive Migration

```go
// New transaction type
type TouchAccount struct {
    Account *url.URL
}

func (TouchAccount) Type() TransactionType { 
    return TransactionTypeTouchAccount 
}

func (x TouchAccount) Execute(st *StateManager, tx *Delivery) error {
    account := st.batch.Account(tx.Account)
    
    // Just accessing the account triggers migration
    _ = account.Directory().Get()
    _ = account.Pending().Get()
    
    // Update timestamp
    account.SetLastModified(time.Now())
    
    return nil
}
```

## Migration Strategy

### Phase 1: Automatic Migration (Immediate)

**What Happens:**
- Deploy new code to all validators
- Any account operation triggers migration
- Both storage formats supported simultaneously

**Account Types & Triggers:**

| Account Operation | Migration Trigger |
|-------------------|-------------------|
| Create sub-account | ✓ Migrates parent ADI |
| Remove sub-account | ✓ Migrates parent ADI |
| Process transaction | ✓ Migrates if pending list accessed |
| Update authorities | ✓ Migrates authority storage |
| Any write operation | ✓ Migrates affected storage |

### Phase 2: Proactive Touch Campaign

**Systematic Migration Using Touch Transactions:**

```go
// Migration coordinator (run by validators or dedicated service)
func RunMigrationCampaign(db Database) {
    // Priority 1: Large ADIs (highest performance gain)
    for _, account := range db.GetADIsWithManySubAccounts() {
        sendTouchTransaction(account)
        time.Sleep(100 * time.Millisecond) // Rate limit
    }
    
    // Priority 2: Active accounts
    for _, account := range db.GetActiveAccounts(last30Days) {
        sendTouchTransaction(account)
        time.Sleep(100 * time.Millisecond)
    }
    
    // Priority 3: Everything else
    for _, account := range db.GetRemainingV1Accounts() {
        sendTouchTransaction(account)
        time.Sleep(100 * time.Millisecond)
    }
}
```

**Touch Transaction Properties:**
- Zero fee (system transaction)
- No user signature required
- Batched for efficiency
- Rate-limited to avoid network stress

### Phase 3: Completion

After all accounts migrated:
1. Remove legacy code paths
2. Remove migration wrappers
3. Simplify BPT hash computation

## Migration Metrics

```go
type MigrationProgress struct {
    // Real-time tracking
    TotalAccounts     uint64
    MigratedAccounts  uint64
    PercentComplete   float64
    
    // Performance metrics
    AvgHashTimeBefore time.Duration  // e.g., 10ms
    AvgHashTimeAfter  time.Duration  // e.g., 1μs
    
    // By component
    DirectoriesMigrated uint64
    PendingMigrated     uint64
    AuthoritiesMigrated uint64
}
```

## Simplified Code Flow

### Before Migration (Current)
```
User Action → Load Full Directory → Modify → Hash All URLs → Update BPT
                    (500KB)                      (O(n))
```

### During Migration (Automatic)
```
User Action → Check Format → Migrate If Needed → Use Chain → Update BPT
                                  (once)                        (O(1))
```

### After Migration (Final)
```
User Action → Use Chain → Update BPT
               (O(1))       (O(1))
```

## Touch Transaction Schedule

### Week 1-2: High-Value Targets
```bash
# Exchanges, major services (est. 100 accounts)
# Performance gain: Massive (these have most sub-accounts)
for account in high_value_list; do
    accumulate touch $account
    sleep 0.1
done
```

### Week 3-4: Active Accounts
```bash
# Recently active (est. 10,000 accounts)
# Performance gain: High (frequently accessed)
for account in $(accumulate query active-accounts --days 30); do
    accumulate touch $account
    sleep 0.1
done
```

### Week 5-8: Bulk Migration
```bash
# Everything else (est. 100,000 accounts)
# Performance gain: Cumulative
for account in $(accumulate query v1-accounts); do
    accumulate touch $account
    sleep 0.1
done
```

## Implementation Checklist

### Core Changes
- [ ] Implement ChainDirectory storage
- [ ] Add AutoMigratingDirectory wrapper
- [ ] Update BPT hash computation for dual-mode
- [ ] Implement TouchAccount transaction type
- [ ] Add migration detection helpers

### Testing
- [ ] Unit tests for migration logic
- [ ] Integration tests with mixed v1/v2 accounts
- [ ] Performance benchmarks before/after
- [ ] Touch transaction rate limiting tests

### Monitoring
- [ ] Migration progress dashboard
- [ ] Performance metrics collection
- [ ] Error tracking for failed migrations
- [ ] Network load monitoring during touch campaign

## Benefits of This Approach

### Simplicity
- No user decisions required
- No new APIs or interfaces
- No voluntary participation needed

### Safety
- Gradual rollout through natural activity
- Touch transactions for inactive accounts
- Both formats work simultaneously

### Performance
- Immediate gains for active accounts
- Systematic migration of all accounts
- No accounts left behind

## Timeline

| Week | Activity | Accounts Migrated |
|------|----------|-------------------|
| 0 | Deploy update | 0 |
| 1-2 | Natural activity migrations | ~1,000 |
| 2-3 | Touch high-value accounts | ~100 |
| 3-4 | Touch active accounts | ~10,000 |
| 5-8 | Touch remaining accounts | ~100,000 |
| 9-10 | Verify completion | All |
| 11-12 | Remove legacy code | - |

## Risk Management

### Minimal Risks

1. **Migration Failure**
   - Mitigation: Keep old data until confirmed
   - Recovery: Retry migration on next access

2. **Network Load from Touch Transactions**
   - Mitigation: Rate limiting, off-peak scheduling
   - Recovery: Pause/resume touch campaign

3. **Unexpected Format Issues**
   - Mitigation: Extensive testing before deployment
   - Recovery: Fix and retry migration

## Success Metrics

### Week 1
- ✓ 10% of active accounts migrated naturally
- ✓ No BPT hash mismatches
- ✓ Performance improvement visible

### Week 4
- ✓ 50% of accounts migrated
- ✓ 100x performance improvement for large ADIs
- ✓ Network stable

### Week 8
- ✓ 95%+ accounts migrated
- ✓ Touch campaign complete
- ✓ Ready for legacy code removal

## Conclusion

This automatic migration approach:

1. **Requires no user action** - happens transparently
2. **Migrates through natural activity** - active accounts first
3. **Uses touch transactions** for complete coverage
4. **Delivers immediate benefits** - performance improves gradually
5. **Maintains full compatibility** - network never stops

The combination of automatic migration on modification and systematic touch transactions ensures complete migration within 8-10 weeks without any user intervention or network disruption.

---

*Design Version: 2.0*  
*Status: Simplified Automatic Migration*  
*Focus: Zero user intervention, complete coverage*