# Consensus and State Optimization Research

**Issue**: #3718
**Date**: March 2026
**Status**: Research

## Executive Summary

This document captures research into optimizing Accumulate's consensus layer and state management. Key findings:

1. **CometBFT's blockstore is unnecessary** - Accumulate's BPT is the authoritative state
2. **Consensus is the throughput bottleneck** - DAG-based alternatives offer 10-25x improvement
3. **Index chains can be eliminated** - Inline BlockMarkers provide simpler timestamp proofs
4. **Directory state doesn't scale** - DirectoryChain reduces ADI state to constant size

---

## Part 1: CometBFT Analysis

### What Accumulate Needs from Consensus

Accumulate runs multiple independent consensus instances:
- **Directory Network (DN)**: Coordinates all partitions
- **Block Validator Networks (BVNs)**: Process transactions in parallel

Each partition needs:
- Transaction ordering (BFT consensus on batches)
- Signed block headers (for anchor proofs)
- Validator signatures (for major block proofs)

### Architecture Clarification

DN and BVNs are **truly independent networks**. Each node runs two consensus instances (DN + one BVN). Cross-partition communication is a **messaging layer** problem, not a consensus problem.

Requirements for cross-partition coordination:
1. **Anchor Delivery** (BVN ↔ DN) - ordering via sequence numbers
2. **Synthetic Transaction Delivery** (any partition → any partition) - same guarantees

### CometBFT's Role Today

| What CometBFT Does | Needed? |
|-------------------|---------|
| Transaction ordering | Yes |
| Block storage (blockstore.db) | **No** |
| Transaction indexing (tx_index.db) | **No** |
| State sync/snapshots | **Broken** |
| Validator signatures | Yes |

### Data Duplication Problem

Every transaction is stored twice:
1. **CometBFT blockstore.db**: Raw blocks and transactions
2. **Accumulate accumulate.db**: Interpreted state and indices

CometBFT's blockstore is 100% redundant because Accumulate's BPT is authoritative.

### Snapshot Unreliability

CometBFT snapshots fail due to hash mismatch after restore (`internal/node/abci/snapshot.go:153`):

```go
if !bytes.Equal(root[:], bptHash) {
    // TODO Can we reset the database?
    return &abci.ResponseApplySnapshotChunk{Result: REJECT_SNAPSHOT}, nil
}
```

Without reliable snapshots:
- Cannot enable `RetainHeight` pruning
- Blockstore grows forever
- New nodes must sync from genesis

---

## Part 2: BPT-Centric Sync Model

### Key Insight

Accumulate's architecture makes CometBFT's block storage fundamentally unnecessary:

```
DN anchors BFT state
  └── BPT root proves all partition state
        └── Chain heads prove all chain entries
              └── Individual records loaded on demand
```

### Layered Sync Model

```
Layer 1: BPT + Major Block Proof
├── Get BPT snapshot
├── Validate with operator signatures (handling rotation)
├── Node can now validate new transactions
└── Immediately joins consensus

Layer 2: Catch-up Sync
├── Pull missing recent data from network
├── Fill gaps since snapshot
└── Fully current with network

Layer 3: Interesting Chains
├── Sync chains the node/user cares about
├── Load from network + historical archives
└── On-demand or background

Layer 4: Full Archival (Optional)
├── Download complete historical archives
├── All past transactions
└── Only needed for archival nodes
```

### What Needs Development

1. **Major Block Proof System**
   - Chain of operator signatures from genesis
   - Handle operator additions/removals over time
   - Cryptographic chain of trust

2. **Historical Data Packaging**
   - Accumulate-native format (not CometBFT blocks)
   - Structured as downloadable archives
   - Verifiable against BPT/chain proofs

---

## Part 3: Consensus Alternatives

### Performance Comparison

| Algorithm | Throughput | Latency | Notes |
|-----------|------------|---------|-------|
| CometBFT | ~10k TPS | 1-6s | Current |
| Bullshark | 100-130k TPS | 2-3s | DAG-based |
| Mysticeti | 200k TPS | 0.5s | Uncertified DAG |
| Shoal++ | 75k TPS | 1.7s | Improved Bullshark |
| Raptr | 260k TPS | <1s | Strong prefix |

### Why DAG-Based Consensus

DAG-based consensus separates data availability from ordering:
- Validators broadcast transactions to DAG structure
- Separate consensus layer orders DAG vertices
- Parallelism enables 10-25x throughput

### Accumulate's Parallelism

The BPT is already designed for parallel execution:

```go
// Insert just adds to pending map - O(1)
func (b *BPT) Insert(key *record.Key, value []byte) error {
    b.pending[key.Hash()] = &mutation{key: key, value: v}
    return nil
}

// Single pass apply at block end
func (b *BPT) executePending() error {
    for _, e := range b.pending {
        b.getRoot().insert(&leaf{...})
    }
}
```

Execution flow:
```
Parallel Chain Execution → pending map → executePending() → BPT Root
```

With parallel execution + DAG consensus, no architectural bottleneck until hardware limits.

---

## Part 4: Chain Structure Optimization

### Current Chain Structure (Per Token Account)

| Chain | Purpose |
|-------|---------|
| MainChain | Transaction entries |
| MainChain.Index | Block/time index |
| SignatureChain | Signature entries |
| SignatureChain.Index | Block/time index |

### Problem: Index Chain Overhead

Each Index chain:
- Adds another chain anchor to BPT
- Requires separate Merkle proof for timestamps
- Creates complex multi-chain proofs

### Solution: Inline BlockMarkers

Replace separate Index chains with inline markers:

```
MainChain: [tx_hash, tx_hash, BlockMarker{block,time,anchor}, tx_hash, ...]
```

**BlockMarker structure:**
```go
type BlockMarker struct {
    Type       byte      // Entry type discriminator
    BlockIndex uint64    // Minor block index
    Timestamp  time.Time // Block timestamp
    AnchorIdx  uint64    // Index in root chain
    EntryCount uint64    // Entries since last marker
}
```

**Distinguishing entries:**
- 32 bytes = transaction/signature hash
- >32 bytes = BlockMarker (unmarshal by length)

### Timestamp Proof Simplification

**Current (complex):**
```
1. Prove tx_hash in MainChain → Merkle proof
2. Find IndexEntry in Index chain
3. Prove IndexEntry in Index chain → Second Merkle proof
4. Correlate indices between chains
```

**Proposed (simple):**
```
1. Prove tx_hash in MainChain → Merkle proof
2. Prove BlockMarker in same MainChain → Same Merkle tree
3. BlockMarker contains timestamp
```

Single Merkle proof covers both transaction and timestamp.

---

## Part 5: DirectoryChain for Sub-Accounts

### Current Problem

Sub-accounts stored as state set in ADI:

```yaml
- name: Directory
  type: state
  collection: set
  dataType: url
```

Every sub-account URL enumerated for BPT hash:

```go
for _, u := range a.Directory().Get() {
    dirHasher.AddUrl(u)
}
```

**Issues:**
- State grows O(n) with sub-accounts
- BPT computation enumerates all URLs
- Large ADIs have expensive updates

### Solution: DirectoryChain

Replace Directory state with a chain:

```yaml
- name: DirectoryChain
  type: other
  dataType: Chain2
  hasChains: true
```

**Benefits:**

| Aspect | State Set | DirectoryChain |
|--------|-----------|----------------|
| State size | O(n) | O(1) constant |
| BPT hash | Enumerate all | Single anchor |
| Add sub-account | Rewrite set | Append entry |
| Membership proof | Full directory | Merkle proof |

### Combined with BlockMarkers

DirectoryChain can use inline BlockMarkers:

```
DirectoryChain: [url_hash, url_hash, BlockMarker{...}, url_hash, ...]
```

"When was sub-account created?" → Merkle proof → BlockMarker → timestamp

---

## Part 6: Implementation Summary

### Quick Wins

1. **Disable CometBFT tx indexing**
   ```go
   d.config.TxIndex.Indexer = "null"
   ```

2. **Stop creating Index chains for user accounts**
   - Executor doesn't use them
   - API searches by hash anyway

### Medium-Term

1. **Implement BlockMarkers**
   - Define BlockMarker type
   - Modify `addChainAnchor` to insert markers
   - Update lookup functions

2. **Implement DirectoryChain**
   - Add DirectoryChain to Account model
   - Modify BPT hash computation
   - Migrate existing directories

3. **Fix snapshot hash mismatch**
   - Investigate BPT computation determinism
   - Enable `RetainHeight` pruning

### Long-Term

1. **Replace CometBFT ordering**
   - Evaluate DAG-based consensus (Bullshark/Mysticeti)
   - Build minimal ordering layer
   - Eliminate blockstore entirely

2. **Major Block Proof System**
   - Operator signature chain
   - BPT-only sync capability

3. **Historical Archives**
   - Accumulate-native format
   - Downloadable block packages

---

## Part 7: Net Impact

### Per Token Account

| Current | Proposed |
|---------|----------|
| MainChain + Index (2 chains) | MainChain with markers (1 chain) |
| SignatureChain + Index (2 chains) | SignatureChain with markers (1 chain) |
| 4 chain anchors in BPT | 2 chain anchors in BPT |

### Per ADI

| Current | Proposed |
|---------|----------|
| Directory state: O(n) URLs | DirectoryChain anchor: O(1) |
| BPT enumerates all sub-accounts | BPT includes single anchor |

### Timestamp Proofs

| Current | Proposed |
|---------|----------|
| Two Merkle proofs | One Merkle proof |
| Cross-chain correlation | Self-contained |

### Throughput Potential

| Current | With DAG Consensus |
|---------|-------------------|
| ~10k TPS per partition | 100k+ TPS per partition |
| Consensus bottleneck | Hardware bottleneck |

---

---

## Part 8: Wallet-Side Routing Optimization

### Current API Flow

```
Wallet → Any Node → RouteAccount() → Forward to BVN → Process → Return → Node → Wallet
```

Every API call:
1. Wallet sends to any node
2. Node computes partition via `RouteTree`
3. Node forwards request to correct BVN
4. BVN processes request
5. BVN returns result to node
6. Node forwards result to wallet

**Issues:**
- Extra hop adds latency
- Node does proxy work (load)
- Wallet has no visibility into network topology

### Routing Is Deterministic

The routing algorithm is simple and deterministic:

```go
func (r *RouteTree) Route(u *url.URL) (string, error) {
    // Check overrides first
    s, ok := r.overrides[u.IdentityAccountID32()]
    if ok {
        return s, nil
    }
    // Route by URL routing number
    return r.RouteNr(u.Routing())
}
```

`url.Routing()` is computed from the URL itself. The prefix tree maps routing numbers to partition IDs.

### Proposed: Wallet Routing Directory

Wallets maintain a local routing directory:

```
Wallet Routing Directory:
├── Routing Table (prefix tree)
├── Partition Endpoints (BVN URLs)
├── Override Map (special accounts)
└── Version/Timestamp
```

**Optimized flow:**
```
Wallet → Compute Partition → Direct to BVN → Process → Return to Wallet
```

### Benefits

| Aspect | Current | With Wallet Routing |
|--------|---------|---------------------|
| Latency | 2 hops | 1 hop |
| Node load | Proxying | None |
| Failover | Node handles | Wallet controls |
| Caching | Node-side | Wallet-side |

### Implementation

1. **Routing Table Distribution**
   - Publish routing table via DN
   - Wallets fetch periodically or on version change
   - Include partition endpoint URLs

2. **Wallet SDK**
   ```go
   type WalletRouter struct {
       table     *RoutingTable
       endpoints map[string][]string  // partition → URLs
   }

   func (w *WalletRouter) Submit(tx *Transaction) (*Result, error) {
       partition := w.Route(tx.Principal)
       endpoint := w.SelectEndpoint(partition)
       return w.client.Submit(endpoint, tx)
   }
   ```

3. **Fallback: Routing Requests**
   - If local routing fails (no table, stale, etc.)
   - Make lightweight routing request:
     ```
     GET /v3/route?account=acc://foo.acme
     → { "partition": "BVN2", "endpoints": [...] }
     ```
   - Then direct call to returned partition
   - Only full proxy as last resort

### API Endpoints

**1. Full Routing Table (for caching)**
```
GET /v3/network/routing
{
  "version": 42,
  "table": { ... },
  "partitions": {
    "BVN0": ["https://bvn0-node1.accumulate.network", ...],
    "BVN1": ["https://bvn1-node1.accumulate.network", ...],
    "Directory": ["https://dn-node1.accumulate.network", ...]
  }
}
```

Wallets cache this and refresh periodically.

**2. Routing Request (lightweight fallback)**
```
GET /v3/route?account=acc://foo.acme/tokens
{
  "account": "acc://foo.acme/tokens",
  "partition": "BVN2",
  "endpoints": [
    "https://bvn2-node1.accumulate.network",
    "https://bvn2-node2.accumulate.network"
  ]
}
```

Used when:
- Wallet doesn't have routing table cached
- Routing table may be stale
- Quick one-off without full table download

### Redirect Pattern (Simplest)

Instead of proxying, nodes return redirect information:

```
1. Wallet → Any Node (may be wrong partition)

2. If wrong partition, node responds:
   {
     "error": "wrong_partition",
     "redirect": {
       "partition": "BVN2",
       "endpoints": ["https://bvn2.example.com", ...]
     }
   }

3. Wallet resubmits → BVN2 directly

4. Wallet caches: acc://foo.acme → BVN2 for future
```

**Benefits:**
- No proxying overhead on nodes
- Wallet learns correct routing over time
- Works without pre-cached routing table
- Simple protocol addition

**Wallet builds routing cache organically:**
- First request to new account → redirect → learn
- Subsequent requests → direct (cached)
- Cache eviction on errors → re-learn

### Fallback Hierarchy

```
1. Local cache hit → direct call
   ↓ miss
2. Any node → redirect response → resubmit direct → cache
   ↓ redirect fails
3. Any node → full proxy (last resort, legacy support)
```

This minimizes latency while ensuring requests always succeed.

### Light Client Integration

For light clients verifying proofs:
- Know which partition holds account
- Direct query to that partition
- Verify proof against DN anchor
- No proxy needed

---

## References

- [DAG Meets BFT](https://decentralizedthoughts.github.io/2022-06-28-DAG-meets-BFT/)
- [Bullshark Paper](https://arxiv.org/pdf/2201.05677)
- [Mysticeti Paper](https://arxiv.org/pdf/2310.14821)
- [Shoal++ Paper](https://www.usenix.org/system/files/nsdi25-arun.pdf)
- Issue #3718: CometBFT Analysis
- Issue #3634: Remove Expensive Merkle Indices
