# DN Height Bug Analysis

## The Bug
**Location:** `internal/api/v3/network.go` line 100

**Current Code:**
```go
func (s *NetworkService) getDnHeight(batch *database.Batch) (uint64, error) {
    c := batch.Account(protocol.PartitionUrl(s.partition).JoinPath(protocol.AnchorPool)).MainChain()
    // ... searches for DirectoryAnchor transactions
}
```

## The Problem
1. Each node runs **BOTH** a Directory Network (DN) partition AND a Block Validator Network (BVN) partition
2. In the current mainnet setup with one BVN (Cyclops), all nodes run both DN and Cyclops
3. The NetworkService is initialized with `s.partition = "Cyclops"` for the BVN service
4. When `getDnHeight()` is called, it searches in `acc://bvn-Cyclops.acme/anchors` 
5. **DirectoryAnchor transactions only exist in `acc://dn.acme/anchors`**, NOT in the BVN's anchor pool
6. Result: The function returns 0 or a cached/default value

## Why It Returns 2460315
The static value 2460315 is likely:
- A cached value from when the code last worked correctly
- A default/fallback value hardcoded somewhere
- The last known DN height before the mainnet was reduced to one BVN

## The Fix
```go
func (s *NetworkService) getDnHeight(batch *database.Batch) (uint64, error) {
    // FIX: Always query the Directory partition, not the local partition
    c := batch.Account(protocol.PartitionUrl(protocol.Directory).JoinPath(protocol.AnchorPool)).MainChain()
    // ... rest of the function remains the same
}
```

## Why The Fix Works
- Changes `s.partition` (which could be "Cyclops") to `protocol.Directory` (always "Directory")
- Now searches in `acc://dn.acme/anchors` where DirectoryAnchor transactions actually exist
- Will find the DirectoryAnchor transactions and return the correct `MinorBlockIndex`

## Testing The Fix
After applying the fix:
1. The API should return the actual DN height that matches what you see on AWS validators
2. The height should increase over time as the DN produces new blocks
3. The value should match the DN's actual minor block index

## Alternative Solutions
If cross-partition database access is restricted:
1. Run separate NetworkService instances for each partition
2. Route DN height queries specifically to the DN's NetworkService
3. Extract DN height from anchor synchronization data in the BVN's database