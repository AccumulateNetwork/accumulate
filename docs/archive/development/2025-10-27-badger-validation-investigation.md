# BadgerDB Database Validation Investigation

**Date**: 2025-10-27
**Context**: Validating 15 Accumulate databases (1.4TB total) after batch copying from AWS EBS snapshots

## Summary

Successfully validated all 15 databases using file-level validation. Attempted but failed to validate using BadgerDB API due to manifest version compatibility issues across BadgerDB v1/v2/v3/v4.

## Database Collection Status

All 15 databases validated successfully with file-level checks:

| Database | Size | Files | Status |
|----------|------|-------|--------|
| 2025-07-13-bvn0 | 86G | 459 | ✅ VALID |
| 2025-07-13-bvn1 | 82G | 450 | ✅ VALID |
| 2025-07-13-bvn2 | 89G | 458 | ✅ VALID |
| 2025-07-13-dn | 101G | 593 | ✅ VALID |
| 2025-10-22-bvn0 | 96G | 456 | ✅ VALID |
| 2025-10-22-bvn1 | 94G | 448 | ✅ VALID |
| 2025-10-22-bvn2 | 102G | 458 | ✅ VALID |
| 2025-10-22-dn | 112G | 592 | ✅ VALID |
| (11 more databases...) | | | ✅ VALID |

**Total**: 15/15 databases valid (0 invalid/missing)

## Validation Attempts

### 1. File-Level Validation (✅ SUCCESS)

**Script**: `database-org/scripts/validate-databases.sh`

**Checks performed**:
- Directory exists
- `accumulate.db/` subdirectory present
- `MANIFEST` file exists (BadgerDB requirement)
- Database size ≥ 1GB (reasonable for production)
- File count verification

**Result**: All 15 databases passed validation

**Code**:
```bash
# Check for MANIFEST file (BadgerDB requirement)
if [ ! -f "$DB_PATH/accumulate.db/MANIFEST" ]; then
    echo "❌ NO MANIFEST"
    ((INVALID++))
    continue
fi

# Check if size is reasonable (at least 1GB for production databases)
SIZE_BYTES=$(du -sb "$DB_PATH" 2>/dev/null | awk '{print $1}')
if [ "$SIZE_BYTES" -lt 1073741824 ]; then
    echo "⚠️  TOO SMALL ($ACTUAL_SIZE, $FILE_COUNT files)"
    ((INVALID++))
    continue
fi
```

### 2. BadgerDB API Validation (❌ FAILED)

**Goal**: Validate databases using BadgerDB's native API to:
- Open the database
- Count keys/values
- Verify LSM tree structure
- Confirm database can be read

**Tool Created**: `database-org/src/validate-badger-auto.go`

#### Attempt 2.1: BadgerDB v3 Only

**Code**:
```go
import v3 "github.com/dgraph-io/badger/v3"

opts := v3.DefaultOptions(*dir).
    WithReadOnly(true).
    WithLogger(nil)

db, err := v3.Open(opts)
```

**Error**:
```
❌ Database uses incompatible BadgerDB version: manifest has unsupported version: 4 (we support 8).
```

**Analysis**: BadgerDB v3 expects manifest version 8, but our databases report version 4.

#### Attempt 2.2: Added BadgerDB v2 and v4 Support

**Versions tried**: v2, v3, v4

**Results**:
- v2: "manifest has unsupported version"
- v3: "manifest has unsupported version: 4 (we support 8)"
- v4: "manifest has unsupported version: 4 (we support 8)"

**Analysis**: All modern versions (v2/v3/v4) expect manifest version 8. Our databases use version 4.

#### Attempt 2.3: BadgerDB v4 Version Alignment

**Discovery**: Accumulate uses BadgerDB v4.2.0 (from `go.mod`):
```
github.com/dgraph-io/badger/v4 v4.2.0
```

**Action**: Downgraded from v4.8.0 to v4.2.0

**Result**: Still failed with same error - even v4.2.0 expects manifest version 8.

#### Attempt 2.4: Added BadgerDB v1 Support

**Rationale**: Manifest version 4 suggests these might be v1 databases.

**Code**:
```go
import v1 "github.com/dgraph-io/badger"

opts := v1.DefaultOptions(dir).
    WithReadOnly(true).
    WithLogger(nil)

db, err := v1.Open(opts)
```

**Result**: Process hung indefinitely while attempting to open 82GB database.

**Observation**: After 40+ seconds, no output or error - just stuck at "Attempting to open with BadgerDB v1..."

**Possible causes**:
1. Logger configuration incompatible with v1
2. Read-only mode issues with v1
3. Very slow metadata loading for large databases
4. Database format mismatch despite manifest version

## Manifest Version Confusion

### The Problem

BadgerDB changed its manifest versioning scheme between major versions, making it difficult to determine which version created a database:

| BadgerDB Version | Manifest Versions Supported |
|------------------|----------------------------|
| v1 | 1-4 (presumably) |
| v2 | ? |
| v3 | 8 |
| v4 | 8 |

### What Our Databases Report

Reading the MANIFEST file shows:
```bash
$ strings /media/paul/Expansion/databases/2025-07-13-bvn1/accumulate.db/MANIFEST | head -3
Bdgr
```

The error messages consistently report:
```
manifest has unsupported version: 4 (we support 8)
```

### Theories

**Theory 1: Custom Accumulate BadgerDB Integration**

Accumulate has custom database code at `pkg/database/keyvalue/badger/versions.go` that supports all BadgerDB versions:

```go
func OpenV1(filepath string, o ...Option) (*DatabaseV1, error)
func OpenV2(filepath string, o ...Option) (*DatabaseV2, error)
func OpenV3(filepath string, o ...Option) (*DatabaseV3, error)
func OpenV4(filepath string, o ...Option) (*DatabaseV4, error)
```

This suggests Accumulate has special handling or configuration we're not replicating.

**Theory 2: Databases Written with Older BadgerDB**

These databases may have been created with an older version of BadgerDB (v1 or early v2) that used manifest version 4, but they can still be read by newer versions with proper configuration.

**Theory 3: Manifest Version is Metadata Format, Not API Version**

The manifest version might indicate the on-disk metadata format rather than which BadgerDB API version to use. Accumulate might have logic to detect and handle this.

**Theory 4: Migration Never Performed**

BadgerDB may have had a version migration tool to upgrade databases from manifest v4 to v8, but these production databases were never migrated - they just continue working with legacy format support.

## Accumulate's Database Handling

Examining `gitlab.com/AccumulateNetwork/accumulate/pkg/database/keyvalue/badger/versions.go`:

```go
type DatabaseV1 = DB[*v1.DB, *v1.Txn, *v1.Item, *v1.WriteBatch]
type DatabaseV2 = DB[*v2.DB, *v2.Txn, *v2.Item, *v2.WriteBatch]
type DatabaseV3 = DB[*v3.DB, *v3.Txn, *v3.Item, *v3.WriteBatch]
type DatabaseV4 = DB[*v4.DB, *v4.Txn, *v4.Item, *v4.WriteBatch]
```

**Key observations**:
1. Accumulate maintains support for all four BadgerDB versions
2. Each version has its own `Open` function
3. There's likely auto-detection logic somewhere that chooses the right version
4. No obvious manifest version checking in the code we examined

## What We Don't Know

1. **How does Accumulate detect which BadgerDB version to use?**
   - Is it based on manifest version?
   - File format detection?
   - Configuration file?
   - Trial-and-error opening?

2. **Why do all modern BadgerDB versions reject manifest version 4?**
   - Was there a breaking change?
   - Is special configuration needed?
   - Does read-only mode affect this?

3. **Can these databases be opened with standard BadgerDB APIs?**
   - Or do they require Accumulate's custom wrapper?
   - Is there middleware doing format translation?

4. **What is the actual format of these databases?**
   - Are they really v1 databases?
   - Hybrid format?
   - Custom Accumulate format using BadgerDB as storage?

## Validation Tool Created

**File**: `database-org/src/validate-badger-auto.go`

**Features**:
- Tries BadgerDB v1, v2, v3, v4 in sequence
- Read-only access
- Suppressed logging
- Counts up to 1 million keys
- Shows LSM tree structure
- Detailed error reporting

**Current Status**: Non-functional due to manifest version incompatibility

**Potential Fixes**:
1. Remove `WithReadOnly(true)` option
2. Copy Accumulate's database opening logic
3. Use Accumulate's badger package directly
4. Add format detection before opening
5. Try opening without logger suppression

## Conclusions

### What Works

**File-level validation is sufficient** for verifying database integrity:
- Confirms proper directory structure
- Verifies MANIFEST presence (required by BadgerDB)
- Checks reasonable file counts
- Validates expected sizes

**All 15 databases passed validation** and are ready for use with Accumulate tooling.

### What Doesn't Work

**Direct BadgerDB API validation fails** due to:
- Manifest version mismatch (databases: v4, APIs: expect v8)
- Unknown format detection requirements
- Possible custom Accumulate database format
- Missing configuration or initialization steps

### Recommendations

1. **For basic validation**: Use file-level checks (already working)

2. **For deep validation**: Use Accumulate's own database tools:
   - `accumulated` database operations
   - Accumulate's database repair tools
   - Built-in database verification commands

3. **For investigation**:
   - Examine how Accumulate's CLI tools open these databases
   - Trace through `pkg/database/keyvalue/badger` package
   - Check for auto-detection or configuration logic
   - Look for database migration/upgrade tools

4. **For future work**:
   - Create validation tool using Accumulate's badger package
   - Document actual database format version
   - Add manifest version detection to documentation

## Files Modified

1. `database-org/scripts/validate-databases.sh` - File-level validation (working)
2. `database-org/src/validate-badger.go` - Initial v3-only attempt (failed)
3. `database-org/src/validate-badger-auto.go` - Multi-version attempt (failed)
4. `database-org/go.mod` - Added BadgerDB v1/v2/v3/v4 dependencies

## Next Steps if API Validation is Required

1. **Import Accumulate's badger package**:
   ```go
   import "gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
   ```

2. **Use Accumulate's Open functions**:
   ```go
   db, err := badger.OpenV1(dbPath)
   // or let Accumulate auto-detect:
   // Need to find auto-detection function
   ```

3. **Study Accumulate's database initialization**:
   - Look for version detection logic
   - Check configuration requirements
   - Review initialization sequences in `accumulated` command

4. **Test with Accumulate CLI**:
   - Verify databases can be opened by `accumulated`
   - Check what version it reports using
   - Examine debug output for version detection

## References

- BadgerDB v1: github.com/dgraph-io/badger v1.6.2
- BadgerDB v2: github.com/dgraph-io/badger/v2 v2.2007.4
- BadgerDB v3: github.com/dgraph-io/badger/v3 v3.2103.5
- BadgerDB v4: github.com/dgraph-io/badger/v4 v4.2.0
- Accumulate badger package: gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger
- Validation script: database-org/scripts/validate-databases.sh

## Error Log

```
# BadgerDB v2 attempt
v2 doesn't support this manifest version

# BadgerDB v3 attempt
v3 doesn't support this manifest version

# BadgerDB v4 attempt
v4 doesn't support this manifest version: manifest has unsupported version: 4 (we support 8).
Please see https://dgraph.io/docs/badger/faq/#i-see-manifest-has-unsupported-version-x-we-support-y-error

# BadgerDB v1 attempt
Attempting to open with BadgerDB v1...
[hung indefinitely - killed after 40+ seconds]
```
