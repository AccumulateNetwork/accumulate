# Soak run 20260904T163512Z

**Purpose:** E8 #4217 CHECK #2, 30 MIN, CHAOS OFF, on issue-4217-two-store-staging @bc11896f8 = check #1 (20260904T140000Z) plus the review fixes: a collected entry is never taken into a run until the proven set covers its hash (Collected mark), a proof is bound to its source by a sequenced sibling, anchor staging bounded (maxAnchorAhead 3600, maxStagedProofBlocks 256), intake store errors fail the block, conflicting proofs counted not fatal, a proven hash accepted whatever proof it carries, snapshot restore rebuilds chain indexes, block ledgers back in snapshots. Check #1 showed the BVN<->BVN leg closed but stalled at 17 min on the Directory: its reconcile pulled 200-entry ranges proven under SOURCE roots (H9, never admissible) into its own mempool while its executor fell behind consensus (C6). H8 and C6 are NOT in this build, so THE SAME SPIRAL IS EXPECTED. THIS RUN ANSWERS: do the review fixes hold for BVN<->BVN (collected entries drain without healer copies; no wedged streams), and what is the Directory's timeline to stall as H8's baseline. WATCH: exec_staged_proofs_total{unbound,refused,conflict} should be ~0; synthetic_anchor_total{collected} climbs then drains; heals BVN1<->BVN2 low; Directory own store and range heals.

| field | value |
|---|---|
| started (UTC) | 2026-09-04T16:35:12Z |
| commit | `bc11896f8a458e98297ccaba644189938cd8dee1` |
| describe | `10k-tps-715-gbc11896f8-dirty` |
| branch | `issue-4217-two-store-staging` |
| uncommitted files | 134 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:392b8585c8d78d0435ef08bbbbe4ae30d5f3fe0a8c732fa71f42470e2ab6afd6` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 30m |
| target TPS | 500 |
| storage | BlockchainDB |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-04T16:58:18Z |
| elapsed | 0.33h |
| driver exit | 0 (clean) |
| dn height | 10 -> 1205 |
| heals | 0 -> 24006 |
| chaos events | 1 |
| monitor samples | 53 |
| seizure | SEIZED at 2026-09-04T16:39:45 :: stuck=0 stuckStream= worst=BVN1->BVN2 gap=104 deliv=2058 undeliv=synthetic BVN2->BVN1 undeliv=867 |
| reconcile pulls (#4073) | 113 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 3396 timed reads, p50 2.2 ms, p95 19.0 ms, p99 41.8 ms, max 121.0 ms (txn read, BVN2, entry 448 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
