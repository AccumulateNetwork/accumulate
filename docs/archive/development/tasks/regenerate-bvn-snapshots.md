# Task: Regenerate Corrupted BVN Snapshots

**Status:** BLOCKING - Must complete before follower deployment
**Priority:** P0
**Created:** 2025-11-27
**Issue:** BVN snapshots have truncated record data causing "not enough data" errors

## Problem Description

The current BVN snapshots fail during restore:
```
Error: load snapshot: failed to restore database: restore Account.acc://...MainChain.States.XXXX: not enough data
```

**Affected files:**
- `accumulate-dual-data/cyclops-genesis-nov17.snap` (2.1 GB)
- `accumulate-dual-data/cyclops-genesis.snap` (2.1 GB)

Both snapshots have the same corruption pattern, suggesting a bug in the snapshot creation process or the original snapshots were created from the same corrupted source.

## Root Cause

The "not enough data" error occurs in `pkg/database/values/helpers.go:22` when decoding record values. This indicates:
- Records section contains truncated binary data
- Length prefix indicates more bytes than available
- Likely caused by interrupted write or buffer issue during snapshot collection

## Solution

Regenerate snapshots from the original Nov 17 validator databases using the `create-snap` tool.

## Prerequisites

1. **Source databases must be accessible:**
   - DN: `/media/paul/Expansion/databases/validator_backup_20251117/extracted/dnn/data/accumulate.db`
   - BVN: `/media/paul/Expansion/databases/validator_backup_20251117/extracted/bvnn/data/accumulate.db`

2. **Build the create-snap tool:**
   ```bash
   cd /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate
   go build -o create-snap ./cmd/create-snap
   ```

## Execution Steps

### Step 1: Verify source databases are accessible

```bash
ls -la /media/paul/Expansion/databases/validator_backup_20251117/extracted/dnn/data/accumulate.db/
ls -la /media/paul/Expansion/databases/validator_backup_20251117/extracted/bvnn/data/accumulate.db/
```

### Step 2: Create output directory

```bash
mkdir -p /mnt/secondary/snapshots-fixed
```

### Step 3: Regenerate DN snapshot

```bash
./create-snap \
  -db /media/paul/Expansion/databases/validator_backup_20251117/extracted/dnn/data/accumulate.db \
  -output /mnt/secondary/snapshots-fixed/directory-genesis.snap \
  -partition Directory \
  -type badger
```

**Expected output:**
- File size: ~2 MB
- Should complete in seconds
- Verify: `go run ./tools/cmd/debug snapshot dump /mnt/secondary/snapshots-fixed/directory-genesis.snap | head -30`

### Step 4: Regenerate BVN snapshot

```bash
./create-snap \
  -db /media/paul/Expansion/databases/validator_backup_20251117/extracted/bvnn/data/accumulate.db \
  -dn-db /media/paul/Expansion/databases/validator_backup_20251117/extracted/dnn/data/accumulate.db \
  -output /mnt/secondary/snapshots-fixed/cyclops-genesis.snap \
  -partition Cyclops \
  -type badger
```

**Expected output:**
- File size: ~2-3 GB
- May take several minutes
- Verify: `go run ./tools/cmd/debug snapshot dump /mnt/secondary/snapshots-fixed/cyclops-genesis.snap | head -30`

### Step 5: Test restore

```bash
# Clean test directory
rm -rf /mnt/secondary/test-restore
mkdir -p /mnt/secondary/test-restore/{dnn,bvnn}/{config,data}

# Copy config templates
cp accumulate-dual-data/dnn/config/{tendermint.toml,accumulate.toml} /mnt/secondary/test-restore/dnn/config/
cp accumulate-dual-data/bvnn/config/{tendermint.toml,accumulate.toml} /mnt/secondary/test-restore/bvnn/config/

# Build accumulated
go build -o /tmp/accumulated-test ./cmd/accumulated

# Test DN restore
/tmp/accumulated-test restore-snapshot --work-dir=/mnt/secondary/test-restore/dnn /mnt/secondary/snapshots-fixed/directory-genesis.snap

# Test BVN restore
/tmp/accumulated-test restore-snapshot --work-dir=/mnt/secondary/test-restore/bvnn /mnt/secondary/snapshots-fixed/cyclops-genesis.snap
```

### Step 6: Replace corrupted snapshots

If both restores succeed:

```bash
# Backup old snapshots
mv accumulate-dual-data/cyclops-genesis-nov17.snap accumulate-dual-data/cyclops-genesis-nov17.snap.corrupted
mv accumulate-dual-data/directory-genesis-nov17.snap accumulate-dual-data/directory-genesis-nov17.snap.old

# Copy fixed snapshots
cp /mnt/secondary/snapshots-fixed/directory-genesis.snap accumulate-dual-data/directory-genesis-nov17.snap
cp /mnt/secondary/snapshots-fixed/cyclops-genesis.snap accumulate-dual-data/cyclops-genesis-nov17.snap
```

## Verification

After regeneration, verify:

1. **Snapshot structure:**
   ```bash
   go run ./tools/cmd/debug snapshot dump accumulate-dual-data/cyclops-genesis-nov17.snap | head -50
   ```
   - Must have Header, Consensus, Bpt, Records sections
   - Consensus section must have validators

2. **Full restore test:**
   ```bash
   # Should complete without "not enough data" error
   /tmp/accumulated-test restore-snapshot --work-dir=/mnt/secondary/test-restore/bvnn accumulate-dual-data/cyclops-genesis-nov17.snap
   ```

3. **Genesis.json verification:**
   ```bash
   cat /mnt/secondary/test-restore/bvnn/config/genesis.json | jq '{chain_id, app_hash}'
   # app_hash must be non-empty
   ```

## Success Criteria

- [ ] DN snapshot regenerated and restores successfully
- [ ] BVN snapshot regenerated and restores successfully
- [ ] Both snapshots have valid consensus sections
- [ ] genesis.json files have correct app_hash values
- [ ] Old corrupted snapshots backed up
- [ ] New snapshots copied to accumulate-dual-data/

## Follow-up

After completing this task:
1. Update PLAN.md to mark P0 as complete
2. Proceed with follower deployment testing
3. Investigate root cause of original corruption to prevent recurrence

## Related

- [Snapshot Creation Guide](../snapshot-creation-guide.md)
- [Plan File](/home/paul/.claude/plans/lazy-growing-frog.md)
- [backupdbs Catalog](/home/paul/go/src/gitlab.com/AccumulateNetwork/backupdbs/CATALOG_SYSTEM_STATUS.md)
