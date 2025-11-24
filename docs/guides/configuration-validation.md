# Accumulate Configuration Validation Guide

**Last Updated:** 2025-11-16

This guide explains common configuration mistakes and how to avoid them, with a focus on the frequently misplaced `partition` field.

---

## Common Bug: Misplaced `partition` Field ⚠️

### **The Problem**

The `partition` field appears in different places in `accumulate.toml` depending on the service type, and users frequently put it in the wrong location.

**This causes:**
- Silent configuration errors
- Services not finding their partition
- Follower failing to start
- Cryptic error messages

### **Root Cause**

There are TWO different uses of the `partition` field:

1. **In `CoreConsensusApp`**: Uses `protocol.PartitionInfo` (structured data with `id` and `type`)
2. **In service types**: Uses a simple `string` partition name

Users mix these up constantly!

---

## The Correct Structure

### ✅ For Consensus Services (CoreConsensusApp)

**CORRECT:**
```toml
[[services]]
  type = "consensus"
  node-dir = "dnn"
  genesis = "../dn-genesis.snap"
  listen = "/ip4/0.0.0.0/tcp/26656"

  [services.app]
    type = "core"

    # Partition info goes HERE inside app
    [services.app.partition]
      id = "Directory"
      type = "directory"

  [services.validator-key]
    type = "raw"
    address = "..."
```

**❌ WRONG - Don't put partition at top level of consensus service:**
```toml
[[services]]
  partition = "Directory"    # ❌ WRONG PLACE!
  type = "consensus"
  ...
```

---

### ✅ For Query/Network/Metrics/Events/Snapshot Services

**CORRECT:**
```toml
[[services]]
  partition = "Directory"    # ✅ CORRECT - partition field at top level
  type = "querier"

[[services]]
  partition = "Directory"
  type = "network"

[[services]]
  partition = "Directory"
  type = "metrics"

[[services]]
  partition = "Directory"
  type = "events"
```

**❌ WRONG - Don't nest partition for these services:**
```toml
[[services]]
  type = "querier"
  [services.app.partition]    # ❌ WRONG - querier doesn't have app!
    id = "Directory"
```

---

## Complete Correct Example

Here's a complete dual-node configuration showing correct partition placement:

```toml
network = "MainNet"

[p2p]
  listen = ["/ip4/0.0.0.0/tcp/16591"]
  bootstrap-peers = [
    "/ip4/23.22.212.106/tcp/16591/p2p/Qm..."
  ]
  [p2p.key]
    type = "raw"
    address = "..."

# ========================================
# Directory Network (DN) Consensus
# ========================================
[[services]]
  type = "consensus"
  node-dir = "dnn"
  genesis = "../dn-genesis.snap"
  listen = "/ip4/0.0.0.0/tcp/26656"
  bootstrap-peers = [...]

  [services.app]
    type = "core"

    # ✅ CORRECT: Partition info for consensus
    [services.app.partition]
      # Note: id and type fields, NOT just partition = "..."

  [services.validator-key]
    type = "raw"
    address = "..."

# ========================================
# DN Storage
# ========================================
[[services]]
  name = "Directory"         # ✅ Storage uses 'name', not 'partition'
  type = "storage"
  [services.storage]
    type = "badger"
    path = "dnn/data/accumulate.db"

# ========================================
# DN Supporting Services
# ========================================
[[services]]
  partition = "Directory"    # ✅ CORRECT: partition field at top level
  type = "querier"

[[services]]
  partition = "Directory"    # ✅ CORRECT
  type = "network"

[[services]]
  partition = "Directory"    # ✅ CORRECT
  type = "metrics"

[[services]]
  partition = "Directory"    # ✅ CORRECT
  type = "events"

# ========================================
# Block Validator Network (BVN) Consensus
# ========================================
[[services]]
  type = "consensus"
  node-dir = "bvnn"
  genesis = "../bvn-genesis.snap"
  listen = "/ip4/0.0.0.0/tcp/26756"
  bootstrap-peers = [...]

  [services.app]
    type = "core"

    # ✅ CORRECT: Partition info for BVN consensus
    [services.app.partition]
      # Note: empty in practice, inferred from config

  [services.validator-key]
    type = "raw"
    address = "..."

# ========================================
# BVN Storage
# ========================================
[[services]]
  name = "Cyclops"           # ✅ BVN name (Cyclops, Apollo, etc.)
  type = "storage"
  [services.storage]
    type = "badger"
    path = "bvnn/data/accumulate.db"

# ========================================
# BVN Supporting Services
# ========================================
[[services]]
  partition = "Cyclops"      # ✅ CORRECT: Use BVN name
  type = "querier"

[[services]]
  partition = "Cyclops"
  type = "network"

[[services]]
  partition = "Cyclops"
  type = "metrics"

[[services]]
  partition = "Cyclops"
  type = "events"
```

---

## Quick Reference Table

| Service Type | Where `partition` Goes | Format | Example |
|-------------|------------------------|--------|---------|
| **consensus** | `[services.app.partition]` | Structured (id + type) | `id = "Directory"` |
| **storage** | Uses `name` instead | String | `name = "Directory"` |
| **querier** | Top level: `partition = "..."` | String | `partition = "Directory"` |
| **network** | Top level: `partition = "..."` | String | `partition = "Directory"` |
| **metrics** | Top level: `partition = "..."` | String | `partition = "Directory"` |
| **events** | Top level: `partition = "..."` | String | `partition = "Directory"` |
| **snapshot** | Top level: `partition = "..."` | String | `partition = "Directory"` |
| **http** | No partition field | N/A | (uses router reference) |
| **router** | No partition field | N/A | |
| **faucet** | No partition field | N/A | |

---

## Validation Checklist

Use this checklist when creating or reviewing `accumulate.toml`:

### For Consensus Services:
- [ ] Does `[[services]]` have `type = "consensus"`?
- [ ] Does it have `[services.app]` section?
- [ ] Does it have `[services.app.partition]` section (can be empty)?
- [ ] Is there NO `partition = "..."` at the `[[services]]` level?

### For Query/Network/Metrics/Events/Snapshot Services:
- [ ] Does `[[services]]` have `partition = "PartitionName"` at top level?
- [ ] Does `partition` value match a storage service `name`?
- [ ] Is there NO nested `[services.app.partition]` section?

### For Storage Services:
- [ ] Does `[[services]]` have `name = "PartitionName"`?
- [ ] Is there NO `partition` field (uses `name` instead)?
- [ ] Does `[services.storage]` have valid `type` and `path`?

---

## Common Errors and Fixes

### Error: Services Can't Find Partition

**Symptoms:**
- "partition not found"
- Services fail to start
- No data being queried

**Cause:** Partition name mismatch or misplaced field

**Fix:**
```toml
# Make sure partition names match exactly

# Storage defines the partition name
[[services]]
  name = "Directory"     # ← This is the partition name
  type = "storage"
  ...

# Services reference that name
[[services]]
  partition = "Directory"  # ← Must match storage name exactly
  type = "querier"
```

---

### Error: Consensus Service Won't Start

**Symptoms:**
- Consensus service fails to initialize
- "invalid configuration"
- Partition type errors

**Cause:** Wrong partition configuration structure

**Fix:**
```toml
# DON'T DO THIS:
[[services]]
  partition = "Directory"  # ❌ WRONG for consensus
  type = "consensus"

# DO THIS INSTEAD:
[[services]]
  type = "consensus"
  [services.app]
    type = "core"
    [services.app.partition]  # ✅ CORRECT (even if empty)
```

---

### Error: Duplicate Services

**Symptoms:**
- "service already exists"
- Multiple services of same type for same partition

**Cause:** Defining same service multiple times

**Fix:**
```toml
# Each partition needs exactly ONE of each service type:
# ✅ CORRECT:
[[services]]
  partition = "Directory"
  type = "querier"

# ❌ WRONG - Don't repeat:
[[services]]
  partition = "Directory"
  type = "querier"  # Duplicate!
```

---

## Partition Names Reference

### Directory Network (DN)
- Always use: `"Directory"`
- Storage name: `"Directory"`
- Type: `"directory"`

### Block Validator Networks (BVN)
Common BVN names (check your network):
- `"Cyclops"` (BVN1)
- `"Apollo"` (BVN2)
- `"Chandelier"` (BVN3)
- Or: `"BVN1"`, `"BVN2"`, etc.

**Important:** BVN names are case-sensitive and must match exactly!

---

## Debugging Configuration Issues

### Step 1: Validate Structure

```bash
# Check TOML syntax
accumulated validate accumulate.toml

# Or use a TOML validator
cat accumulate.toml | toml-lint
```

### Step 2: Check Partition Names

```bash
# Extract all partition definitions
grep -E "partition = |name = " accumulate.toml

# Should show:
#   name = "Directory"      (storage)
#   partition = "Directory" (services)
#   name = "Cyclops"        (storage)
#   partition = "Cyclops"   (services)
```

### Step 3: Verify Service Types

```bash
# List all services and their types
grep -B2 "type = " accumulate.toml | grep -E "services|type"

# Each partition should have:
# - 1 consensus service
# - 1 storage service
# - 1 querier service
# - 1 network service
# - 1 metrics service
# - 1 events service
```

---

## Prevention: Configuration Templates

Use these templates to avoid mistakes:

### Template: Dual Node Follower

```toml
network = "MainNet"

[p2p]
  listen = ["/ip4/0.0.0.0/tcp/16591"]
  [p2p.key]
    type = "raw"
    address = "YOUR_NODE_KEY"

# DN Services (copy this block)
[[services]]
  type = "consensus"
  node-dir = "dnn"
  genesis = "../dn-genesis.snap"
  listen = "/ip4/0.0.0.0/tcp/26656"
  [services.app]
    type = "core"
    [services.app.partition]
  [services.validator-key]
    type = "raw"
    address = "YOUR_VALIDATOR_KEY"

[[services]]
  name = "Directory"
  type = "storage"
  [services.storage]
    type = "badger"
    path = "dnn/data/accumulate.db"

[[services]]
  partition = "Directory"
  type = "querier"

[[services]]
  partition = "Directory"
  type = "network"

[[services]]
  partition = "Directory"
  type = "metrics"

[[services]]
  partition = "Directory"
  type = "events"

# BVN Services (copy this block, adjust BVN name)
[[services]]
  type = "consensus"
  node-dir = "bvnn"
  genesis = "../bvn-genesis.snap"
  listen = "/ip4/0.0.0.0/tcp/26756"
  [services.app]
    type = "core"
    [services.app.partition]
  [services.validator-key]
    type = "raw"
    address = "YOUR_VALIDATOR_KEY"

[[services]]
  name = "Cyclops"  # ← CHANGE THIS to your BVN name
  type = "storage"
  [services.storage]
    type = "badger"
    path = "bvnn/data/accumulate.db"

[[services]]
  partition = "Cyclops"  # ← CHANGE THIS to match above
  type = "querier"

[[services]]
  partition = "Cyclops"
  type = "network"

[[services]]
  partition = "Cyclops"
  type = "metrics"

[[services]]
  partition = "Cyclops"
  type = "events"
```

---

## Summary

### The Golden Rules:

1. **Consensus services**: Put partition info inside `[services.app.partition]` (can be empty)
2. **Query/Network/Metrics/Events/Snapshot**: Put `partition = "Name"` at top level
3. **Storage**: Use `name = "Name"` instead of partition
4. **Partition names must match exactly** between storage `name` and service `partition`
5. **Case-sensitive**: "Directory" ≠ "directory", "Cyclops" ≠ "cyclops"

### Prevention:

- ✅ Use configuration templates
- ✅ Validate with checklist
- ✅ Test configuration before deployment
- ✅ Match partition names exactly
- ✅ One service of each type per partition

---

## See Also

- [QUICK_START_LOCAL_BACKUP.md](QUICK_START_LOCAL_BACKUP.md) - Deployment guide
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - Error solutions
- [cmd/accumulated/run/schema.yml](../cmd/accumulated/run/schema.yml) - Configuration schema
- [.nodes/bvn1-1/accumulate.toml](../.nodes/bvn1-1/accumulate.toml) - Working example

---

**If you keep getting configuration errors, compare your file line-by-line with a working example from `.nodes/`!**
