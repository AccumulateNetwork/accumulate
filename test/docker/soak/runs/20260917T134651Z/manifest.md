# Soak run 20260917T134651Z

**Purpose:** 1 h at 300 tps, chaos off. The first Accumulate run since the measurement disk was trimmed (1.5 TiB discarded 2026-09-17 07:15; every earlier soak shared a drive that stalled every fsync on the box for 30-40 s at a time under sustained writes). Store is BlockchainDB 8751863 (puts never compact, #92); the heap/files dyna-heap work is NOT in this build. Question: with the drive fixed, does the serial executor hold 300 tps for an hour with execution lag near zero and no refusals? The 20260916T182706Z run at the same rate held 301 tps with 6 refusals to minute 18, then a knee at 21 min, on the untrimmed disk and with other load sharing the box. Watch: accepted tps vs 300, execution lag, refusals, block-time tail; heal entries must stay zero.

| field | value |
|---|---|
| started (UTC) | 2026-09-17T13:46:51Z |
| commit | `771acfb8e0abd874798a0d010a4da4f073f966e0` |
| describe | `10k-tps-721-g771acfb8e-dirty` |
| branch | `dagbft-integration` |
| uncommitted files | 292 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:a6f9608e9d0fec207201f5bc815791894b059548c99fec52495377d49d3c22f3` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| fault model | none (CHAOS=off) |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 1h |
| target TPS | 300 |
| storage |  (docker-network.yml) |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/` (soak.conf + override.conf, the compose and network files). Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-17T14:49:24Z |
| elapsed | 1.01h |
| driver exit | 0 (clean) |
| dn height | 10 -> 3657 |
| heals | 0 -> 0 |
| chaos events | 1 |
| monitor samples | 113 |
| seizure | SEIZED at 2026-09-17T14:03:24 :: stalled stream, undelivered for 20 polls :: stuck=n/a stuckStream= worst=BVN1->Directory gap=0 deliv=3968 undeliv=synthetic BVN2->BVN1 undeliv=443 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 3 |
| read-back probe | Whole run: 15720 timed reads, p50 1.3 ms, p95 8.4 ms, p99 34.1 ms, max 274.7 ms (txn read, Directory, entry 2201 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.

## Result — the hour held at 300 tps

**Question.** With the measurement disk trimmed for the first time (1.5 TiB
discarded at 07:15Z; see the BlockchainDB tree's `~/infrastructure/disk-trim.md`),
does the serial executor hold 300 tps for an hour with execution lag near zero
and no refusals?

**Answer: it holds the rate for the whole hour.** 1,080,002 transactions
generated at 300.0-300.9 tps against a target of 300, every followed
transaction delivered, zero heals, zero wedges, zero reconcile pulls, and
15,720 read-back probes with nothing failed or timed out. Directory 10 ->
3,657. Blocks stayed at exactly 1.0 s on all three partitions from the first
minute to the last. Execution lag never exceeded 2 blocks.

| t (load) | tps | refused (cum.) | exec lag | heals | s/blk |
|---|---|---|---|---|---|
| 1-32 min | 300.9 | 4 | 0-1 | 0 | 1.0 |
| 35 min | 300.5 | 8,518 | 1 | 0 | 1.0 |
| 45 min | 300.6 | 8,518 | - | 0 | 1.0 |
| 52 min | 300.5 | 13,504 | 2 | 0 | 1.0 |
| 55 min | 299.9 | 32,133 | - | 0 | 1.0 |
| 60 min | 300.0 | 32,871 | - | 0 | 1.0 |

For contrast, 20260916T182706Z at the same rate had 21,525 refusals and lag
13 by minute 24 -- on the untrimmed disk and sharing the box with other work.

**The refusals are the lag gate, in four bursts, and they cost no throughput.**
32,871 of 1,080,002 submissions (3.0%) were refused with "worker refusing user
submissions: execution is lagging consensus". The load generator retries a
refused pick and only loses a tick when every attempt is refused, so the
offered rate never dropped below 299.9. Nothing was rejected (0) and nothing
was stranded. The bursts, from the node logs:

| burst | partition |
|---|---|
| 14:22 | BVN2 only |
| 14:39 | BVN2 only |
| 14:42-14:44 | BVN2 and BVN1 |
| 14:47 | BVN2 only |

**BVN2 is the loaded side and refuses first.** At minute 35, BVN2's four nodes
had each read ~70 GB and written ~75 GB at 55-72% CPU, against ~35 GB read and
~54 GB written at 40-49% on BVN1's. BVN2 produces the heaviest cross-partition
stream (BVN2->BVN1, 102,742 sent by minute 35, against 38,423 the other way).
BVN2 carried 1-2 blocks of execution lag while BVN1 carried 0. That asymmetry
is the shape recorded for the store's global read lock and segment walk
(BlockchainDB #50); this run says it is still what bends first, now that the
drive underneath is no longer the limit. Host I/O pressure (`/proc/pressure/io`
full) drifted 8.5% -> 15.1% over the hour, against 24-34% on the 8-shard
500 tps run that was compacting from the commit path.

**Store under test.** BlockchainDB `8751863` (puts never compact, #92). The
heap/files dynamic-layer work (BlockchainDB `dyna-heap`) is NOT in this build.

**Two reporting defects, neither affecting the run.**

1. `seizure` records SEIZED at 14:03:24 on "stalled stream, undelivered for 20
   polls", synthetic BVN2->BVN1, undeliv ~450. The stream was not stalled:
   `deliv` climbed 2,993 -> 3,968 across the same five polls and `gap=0`. At
   300 tps a healthy pipeline always has a few hundred in flight, so
   "undelivered > 0 for 20 consecutive polls" fires on a working network. The
   watchdog needs to test that `deliv` is not advancing, not that `undeliv` is
   non-zero.
2. `chaos events | 1` although `CHAOS=off`. The only line in `chaos.log` is
   `DISABLED for this run (CHAOS=off)`, which the counter counts as an event.
   REPORTING-SPEC 1 exists because a manifest must not record fault injection
   that did not happen.

**Read-back probe.** 15,720 timed reads: p50 1.3 ms, p95 8.4 ms, p99 34.1 ms,
max 274.7 ms (a txn read on the Directory, 2,201 blocks old). 0 failed, 0 timed
out, 0 refused by the query gate.

**Node footprint.** RSS 616-860 MiB per node against `mem_limit` 2048 MiB and
GOMEMLIMIT 1700 MiB; no node approached the ceiling. Host CPU p50 529%, max
936% across the 8 nodes plus bootstrap.
