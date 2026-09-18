# Soak run 20260903T121819Z

**Purpose:** bcdb 12h/500tps at 1s BLOCKS (was 3s) on 6f23c59b6. Faster pacing so failures arrive sooner: the two runs that wedged BVN2 took 8 and 23 minutes at 3s, and a soak exists to reach failures. init network --block-interval now pins the interval into every generated node config, mirroring --database, and soak.sh records it in the manifest. Under test: the primary deadlock fix (cleanupOldHeaders held pendingMu and the pin release re-entered it -- a deadlocked primary stops advancing rounds while gossip flows and containers report healthy, which is what wedged BVN2 both times and why chasing the synthetic stream never converged); the batch pin with its lifecycle tested end-to-end through OnHeaderReceived and mutation-checked both ways; submit no longer refusing work (crossing a pending boundary seals synchronously, keeping the envelope); #4189 durable unhashed staging; #4201 heal cadence; two bcdb read caches; tally as counters; 2048m/1700MiB. WATCH: BVN2 block production must not stop; recv-deliv never pinned at 4096; BVN1 received-from-BVN2 tracks BVN2 produced-for-BVN1.

| field | value |
|---|---|
| started (UTC) | 2026-09-03T12:18:19Z |
| commit | `6f23c59b631d82b851d72404d660720bfe69c6a1` |
| describe | `10k-tps-665-g6f23c59b6-dirty` |
| branch | `issue-4189-staging-out-of-account` |
| uncommitted files | 10 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:b220385fd294a6aa43d4212e2ec7aa8f890c012e370b992825654e5c6804032a` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | on |
| target duration | 12h |
| target TPS | 500 |
| storage | BlockchainDB |
| block interval | 1s |
| memory budget | mem_limit 1536m, GOMEMLIMIT 1200MiB |

Config as run is frozen in `config/`. Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-09-03T12:36:54Z
- reason: stalled 246s: BVN1,BVN2,Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-03T12:36:55Z |
| elapsed | 0.26h |
| driver exit | 143 (FAILED) |
| dn height | 19 -> 655 |
| heals | 8 -> 39085 |
| chaos events | 4 |
| monitor samples | 4 |
| seizure | SEIZED at 2026-09-03T12:26:21 :: stuck=0 stuckStream= worst=BVN1->BVN2 gap=111 deliv=6768 undeliv=synthetic BVN2->BVN1 undeliv=3611 |
| reconcile pulls (#4073) | 458 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 1620 timed reads, p50 75.5 ms, p95 628.0 ms, p99 1210.6 ms, max 8040.4 ms (chain read, Directory, entry 637 blocks old); 12 failed, 12 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260903T123448Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
