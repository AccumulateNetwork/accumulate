# Review — run 20260905T032333Z

500 tps, 45 minutes, chaos off, BlockchainDB, 1 s blocks, `issue-4193-producer-cache`
@bff07d2d6: H1 (dispatch and the sequencer read the producer cache), D7 (a first
write reads nothing), D8 (element index written blind except for unique chains),
R3 (the adapter never walks for a shallow miss; one deep reader for referenced
pending transactions), E10 (staging is memory). Before C6 (#4215) and before H8
(#4216). A 45-minute run is a measurement, not a stability claim.

Compared against `20260904T221627Z` (40 min, same load, `issue-4219-absence-reads`
at R1), which had 113.8M history walks on the eight BVN stores, 99.2% of them
proving a key absent before its first write.

## What the run answers

**The executor reads no history.** `fallbackWalks` is 0 on all 16 stores: no
shallow miss walked the permanent layer. Every read that reached history came
from the API (`internal/api/v3`), none from `internal/core/execute`,
`internal/database` or the conductor. The 165M shallow misses on the BVN stores
are the dynamic-layer absence checks the manifest expected (Transaction.Status
16%, Signatures 11.5%, Transaction.Main 10%, Payments, Votes, Produced, Cause,
History), answered by the dynamic layer without a walk; the main-chain
ElementIndex guard read (3.3%) is E9's, gone in the next build.

**Synthetic cache: zero misses**, 2,808 blocks held on every BVN node at the end
(the horizon is 3,600). Dispatch and the sequencer never touched the store.

**Heals 0 → 0**, no stall (stallkill and wedgewatch: worst stall 0 s all run),
driver exit clean, Directory height 10 → 2,843, load generator 1,323,440 of
1,350,000 generated at 497 tps, 11 rejected.

## What still reads history — all API

| store | hits | misses | reader |
|---|---|---|---|
| Directory (8 nodes) | 17,736,988 | 6,373 | `NetworkService.getDnHeight` loading anchor bodies (`Message.Main`), 576 hits per key |
| Directory | 0 | 13,217 | `queryAccount` Pending for accounts that do not exist |
| Directory | 0 | 7,775 | `getMajorHeight` MajorBlockChain.Head |
| BVN (8 nodes) | 0 | 699,085 | `queryAccount` Main for accounts that do not exist |
| BVN | 0 | 276,899 | `queryAccount` Pending, same |
| BVN | 0 | ~12,800 each × 11 shapes | `loadMessage`/`tryLoad` for transaction ids not yet executed |
| BVN | 11,851 | 672 | `Transaction.(hash).Main`, 8 keys (v1 shape) |

Two things follow.

1. `getDnHeight` was the largest reader of history in the network: a status
   poll per submission walked the Directory anchor pool's main chain from the
   head loading anchor bodies until it found a DirectoryAnchor. 890 reads a
   second per node, all for the same few recent keys. Fixed after this run: on
   the Directory it returns the system ledger's index, one read of mutable
   state (`internal/api/v3/network.go`).
2. An API query for an account or a transaction that does not exist walks the
   whole history to prove the absence — about 1M such walks in 45 minutes on
   the BVN stores, 360 a second across eight nodes, from the load generator's
   pre-checks. The API batch is deep by design (R3), so a miss is a full walk.
   Existence is answered by the BPT and by the dynamic layer's recent state;
   an absence should be decided there before the deep reader is asked. Not
   fixed here; it is the next reader item under #4219.

## BVN2 still falls behind — and it is memory, not reads

Block commits per store (from `storage-stats.csv`):

| time | BVN1 | BVN2 | Directory |
|---|---|---|---|
| 03:36 (+10 min) | 550 | 550 | 550 |
| 03:39 | 750 | 700 | 750 |
| 03:48 | 1,250 | 1,100 | 1,300 |
| 03:57 | 1,800 | 1,350 | 1,800 |
| 04:06 | 2,250 | 1,550 | 2,350 |
| 04:11 (end) | 2,450 | 1,650 | 2,650 |

BVN2 leaves the pace at minute ~13, the same minute as in `20260904T221627Z`,
with every history walk removed. Its block rate falls to 0.28/s by the end.

Profiles at minute 47, `acc-bvn2-val1` against `acc-bvn1-val1`, same binary
(`profiles/`):

| | BVN2 | BVN1 |
|---|---|---|
| CPU in GC (20 s profile) | 26.1 s of 41.2 s sampled, 63% | 9.3 s of 26.3 s, 35% |
| heap in use | 1,021 MB | 538 MB |
| RSS / heapInuse at the end (`mem.csv`) | 1.5 GB / 1.35 GB | 1.05 GB / 0.93 GB |
| GC cycles per second | 1.1 | — |
| heap reachable from `ProduceBlock` | 808 MB | 365 MB |
| of which `produceSyntheticInto` (cache copies) | 134 MB | 70 MB |
| the block's envelopes (`Envelope.UnmarshalBinary`) | 143 MB | 49 MB |
| live allocations under `executeTransaction` + `callSignatureExecutor` | 182 + 198 MB | 60 + 62 MB |

BVN2 produces about twice the synthetics BVN1 does — the load's payers live on
BVN2, so it executes the credit purchases and produces the deposits — and is
the slower partition for it. Once its executor lags consensus, every round it
executes carries more transactions; the block's live state grows with it; the
live heap approaches GOMEMLIMIT (1,700 MiB); GC runs every second over a 1 GB
heap and takes 1.3 cores; execution slows further. A feedback loop, not a
leak: BVN1 at the same rate is flat at 35% GC. Filed as
[#4220](https://gitlab.com/accumulatenetwork/accumulate/-/work_items/4220).
C6, merged after this build, caps the lag at 8 blocks; the next soak says
whether that bound alone stops the spiral. The cache's horizon-based retention
(DIFFERENCES H1) and the per-block retained state are the other two levers.

The seizure watch tripped at 03:28:58 on a BVN1→BVN2 gap of 1,173 undelivered
synthetics: BVN2's executor was already behind BVN1's production two minutes
in. It is the same asymmetry, seen from the stream.

## Memory

Not flat. BVN1 nodes 77 → 1,050 MiB RSS over 45 minutes, BVN2 nodes
77 → 1,500 MiB. The producer cache grows until the horizon (an hour) and
should plateau after it; nothing in this run reached that point. The
BlockchainDB immutable cache holds 52–56 MB on both.

## Read-back probe

11,820 timed reads, p50 12.4 ms, p95 187 ms, p99 481 ms, max 4.09 s (a
Directory transaction 2,311 blocks old); 0 failed, 0 timed out, 0 refused.

## Not verified here

- Whether C6's bound stops BVN2's spiral, and whether the cache plateaus at the
  horizon: both need the next run on the current tip (C6 + E9 + H8).
- Where the load's asymmetry comes from exactly (which accounts route to
  BVN2); inferred from the synthetic production ratio, not measured.
- The API absence-walk fix is not made; only counted.
