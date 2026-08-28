# Soak run 20260822T061030Z

**Purpose:** 12h chaos soak on a9ea422c1. Consensus: #4125 retention + re-delivery skip, #4128 committed batches stay fetchable. Loadgen: #4130 fund-before-advertise, #4129 lock guard, bootstrap phase bounded by one deadline with a funded/stuck summary. Harness: soakmon supervised + line-buffered + heartbeat + names its killing signal; watchdogs log decisions; first failure and first skip of every action type logged. Open question this run should answer: why only a handful of the 100 bootstrap deposits execute (DN stops at 553 every run) — note #4131, DI submit returns TxID nil so nothing can be followed by id.

| field | value |
|---|---|
| started (UTC) | 2026-08-22T06:10:30Z |
| commit | `a9ea422c1cface498efee0f5b243f4833b636e10` |
| describe | `10k-tps-427-ga9ea422c1-dirty` |
| branch | `issue-4105-collection-proof-delivery` |
| uncommitted files | 2 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 12h |
| target TPS | 10 |

Config as run is frozen in `config/`. Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-08-22T06:18:53Z
- reason: stalled 370s: BVN1,BVN2,BVN3,Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-22T06:18:53Z |
| elapsed | ?h |
| driver exit | 143 (FAILED) |
| dn height | 169 -> 553 |
| heals | 4 -> 8 |
| chaos events | 0 |
| monitor samples | 2 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 0 |
| wedge captures (#4125) | 1 wedge-20260822T061620Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
