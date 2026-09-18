# Soak run 20260904T140000Z

**Purpose:** E8 #4217 CHECK, 30 MIN, CHAOS OFF, on issue-4217-two-store-staging = run #6 code plus two-store staging: proofs name their Directory anchor block and wait in anchor staging until it executes; an unproven synthetic is collected (held at its number) instead of recorded pending outside staging; the proven set is unhashed and refuses conflicting proofs. Run #6 (20260904T035906Z) lost a third of every stream to the missing-anchor leg and the healer carried it at 230 heals/s. THIS RUN ANSWERS ONE QUESTION: with the leg closed, do heals fall to ~0 at 500 tps? WATCH: heals total (should stay near zero); exec_staged_proofs_total{staged,validated,disproved}; exec_synthetic_anchor_total{collected,proven}; block times; refusal. Not a steady-state claim: 30 minutes proves nothing about growth.

| field | value |
|---|---|
| started (UTC) | 2026-09-04T14:00:00Z |
| commit | `7241e2b0b32bc5c8a4996a0fff37b24008e841c0` |
| describe | `10k-tps-707-g7241e2b0b-dirty` |
| branch | `issue-4217-two-store-staging` |
| uncommitted files | 119 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:79838c923d0600039c0095334d9adaec5f07bbdb69459e30d2d46369502c6d0d` |
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

## Stopped early by stallkill

- stopped (UTC): 2026-09-04T14:21:27Z
- reason: stalled 241s: Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-04T14:21:27Z |
| elapsed | 0.29h |
| driver exit | 143 (FAILED) |
| dn height | 10 -> 603 |
| heals | 0 -> 124132 |
| chaos events | 1 |
| monitor samples | 47 |
| seizure | SEIZED at 2026-09-04T14:04:57 :: stuck=0 stuckStream= worst=BVN2->Directory gap=1137 deliv=576 undeliv=synthetic BVN2->BVN1 undeliv=889 |
| reconcile pulls (#4073) | 676 |
| stalled channels at end | 4 |
| read-back probe | Whole run: 2778 timed reads, p50 3.0 ms, p95 17.0 ms, p99 40.9 ms, max 88.5 ms (txn read, BVN1, entry 740 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed). |
| wedge captures (#4125) | 1 wedge-20260904T141901Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`.
