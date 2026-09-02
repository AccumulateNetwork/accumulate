# Soak run 20260901T054802Z

**Purpose:** first soak on the sharded store: 8 shards, N=20, windowed perm reads with deepFallbacks counting, pack tier every 1000 blocks (BlockchainDB ec4e2e6)

| field | value |
|---|---|
| started (UTC) | 2026-09-01T05:48:02Z |
| commit | `b05b0937944c269eeaf5571e11d213bbe329470f` |
| describe | `10k-tps-619-gb05b09379` |
| branch | `feat/sharded-store` |
| uncommitted files | 2 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:e7e0d8100e64f14974a545ed96f4091b0e838f8a2e425f8be26b668746978958` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | on |
| target duration | 4h |
| target TPS | 500 |
| storage | blockchainDB |
| memory budget | mem_limit 1536m, GOMEMLIMIT 1200MiB |

Config as run is frozen in `config/`. Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-09-01T06:02:17Z
- reason: stalled 247s: BVN2,Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-01T06:02:17Z |
| elapsed | 0.19h |
| driver exit | 143 (FAILED) |
| dn height | 8 -> 210 |
| heals | 0 -> 3161 |
| chaos events | 4 |
| monitor samples | 3 |
| seizure | SEIZED at 2026-09-01T05:57:32 :: stuck=0 stuckStream= worst=BVN2->BVN1 gap=593 deliv=35679 undeliv=synthetic BVN2->BVN1 undeliv=4968 |
| reconcile pulls (#4073) | 219 |
| stalled channels at end | 3 |
| read-back probe | Whole run: 1427 timed reads, p50 1.4 ms, p95 30.8 ms, p99 249.1 ms, max 588.0 ms (txn read, BVN2, entry 126 blocks old); 0 failed, 0 timed out (8s), 1 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260901T060013Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
