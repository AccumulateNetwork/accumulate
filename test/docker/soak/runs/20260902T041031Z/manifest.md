# Soak run 20260902T041031Z

**Purpose:** SOAK-12H-500-URLDYNA: 12h CHAOS soak at 500 tps on the sharded store (8 shards, N=20, BlockchainDB ec4e2e6) with Account(U).Url routed to the DYNAMIC layer (c37c2eeb0). The previous run 20260901T054802Z seized at 14 min: every BVN engine logged 96,303 deep fallbacks over 200 commits, ALL of them Account.(url).Url, ~482 history walks a block per node, and it was the only shape falling back on the BVN engines at all. The record is written once so isWriteOnce called it permanent, but the permanent layer is read through a window and the executor reads that record on every touch of an account, so it aged out at once and every read after walked history. Acceptance: deepFallbacks for Account.(url).Url ZERO in storage-stats; BVN2 CPU near BVN1's rather than 3-4x it; no seizure; blocks 3.0 s; heals return to 0 after chaos; maintenanceErrors 0; probe reads of old entries still answer.

| field | value |
|---|---|
| started (UTC) | 2026-09-02T04:10:31Z |
| commit | `c37c2eeb09e633d80e2f9069cd377be2f49d3729` |
| describe | `10k-tps-620-gc37c2eeb0-dirty` |
| branch | `feat/sharded-store` |
| uncommitted files | 4 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:e37efac4f239b637d2684f8b282fd6d94351ccdb92e12865a7deb7bca0192192` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | on |
| target duration | 12h |
| target TPS | 500 |
| storage | blockchainDB |
| memory budget | mem_limit 2560m, GOMEMLIMIT 2GiB |

Config as run is frozen in `config/`. Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-09-02T04:34:21Z
- reason: stalled 245s: BVN2,Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-02T04:34:22Z |
| elapsed | 0.33h |
| driver exit | 143 (FAILED) |
| dn height | 8 -> 387 |
| heals | 0 -> 9317 |
| chaos events | 4 |
| monitor samples | 5 |
| seizure | SEIZED at 2026-09-02T04:23:19 :: stuck=0 stuckStream= worst=BVN1->BVN2 gap=417 deliv=16085 undeliv=synthetic BVN2->BVN1 undeliv=2074 |
| reconcile pulls (#4073) | 287 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 3599 timed reads, p50 1.9 ms, p95 60.7 ms, p99 397.2 ms, max 8040.5 ms (chain read, BVN2, entry 253 blocks old); 8 failed, 8 timed out (8s), 1 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260902T043216Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.

## Verdict: the URL fix worked; the wedge is elsewhere and is not storage

The change under test did exactly what it was for. Deep fallbacks per BVN
engine, against the previous run:

| run | deep fallbacks on a BVN engine | shapes |
|---|---|---|
| 20260901T054802Z | 96,303 over 200 commits | `Account.(url).Url` only |
| this run | **5** over 400 commits | `AnchorChain.directory.Root.ElementIndex` only |

`Account.(url).Url` is gone entirely. The ~482 history walks a block per node
are gone with it, and the CPU asymmetry that made BVN2 the partition that broke
first went with them: under load at 15 minutes BVN1 ran 130-221% and BVN2
181-192%, where the previous run had BVN2 at 37-57% against BVN1's 14-19%.

**And the run still seized, at 20 minutes.** BVN2 stalled at height 320 while
the Directory (362) and BVN1 (353) held ~3.1 s/block. Seizure was on a
synthetic sequence gap, `BVN1->BVN2 gap=417` — the direction flipped from the
previous two runs (BVN2->BVN1, gaps 514 and 593), which is worth noting: it is
the pair that fails, not one direction of it.

**It is not the storage write path.** Goroutines captured from two BVN2 nodes
during the stall (`stall-043045Z/`) contain **zero** `semacquire` — nothing
waiting on any lock — and the block-production loops are parked in `[select]`
waiting for committed groups. That is consensus not delivering, the shape of
run 20260831T070855Z, NOT the fsync-inside-seal shape of 20260901T054802Z.

The standing condition is unchanged, because nothing has addressed it: BVN2
carries the load. Synthetics BVN2->BVN1 74,434 against BVN1->BVN2's 28,166
(2.6x), and BVN2->Directory 17,171 against BVN1->Directory's 3,919 (4.4x).

## CPU, profiled rather than guessed at

30 s profile of `acc-bvn2-val1` under load (`cpu-20260902T041738Z/`):

| item | share |
|---|---|
| `syscall.Syscall6` flat | 18.45% |
| — of which `pread` | 16.5% of total |
| — of which `segment.bloomTest` -> `os.File.ReadAt` | **71.8% of the preads** |
| `segment.lookup` | 18.6% of the preads |
| `segment.readValue` (actual data) | 8.9% of the preads |
| GC (`scanobject`+`mallocgc`, cum) | ~11% |
| ed25519 `feMul`/`feSquare` | ~6% |

One CPU-second in six is `pread`, and nearly three quarters of that is fetching
bloom filters from disk in order to test them. `Bloom.Test` itself is 1.17%:
the cost is the I/O to get the filter, not the test. Actual value reads are
under 9% of it. This is a separate thing from the fallbacks that were fixed --
that was deep HISTORY walks; this is the ordinary segment read path -- and it
is the shape BlockchainDB#56 and run 20260831T045856Z's "23% of CPU in bloom
probing" were about, returned in a different form.

## Not proven

Whether the bloom-read cost grows with chain age. The profiler was armed to
sample every 15 minutes and the run died before a second sample under load.
