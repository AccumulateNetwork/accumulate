# Soak run 20260824T065208Z

**Purpose:** throughput probe 4: 64 submitters, 800 tps target, leveldb bloom+caches. Rate ladder tonight: 9.7 -> 84 -> 212 -> 436. Chaos OFF until defect 7

| field | value |
|---|---|
| started (UTC) | 2026-08-24T06:52:08Z |
| commit | `6e6bddc6c2584fdd1c0a6641ad32b48606eacb82` |
| describe | `10k-tps-503-g6e6bddc6c-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 49 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 12h |
| target TPS | 800 |

Config as run is frozen in `config/`. Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-08-24T09:37:38Z
- reason: stalled 240s: BVN2,Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-24T09:37:38Z |
| elapsed | 2.68h |
| driver exit | 143 (FAILED) |
| dn height | 8 -> ? |
| heals | 0 -> 15188 |
| chaos events | 2 |
| monitor samples | 33 |
| seizure | SEIZED at 2026-08-24T07:03:05 :: stuck=0 stuckStream= worst=BVN1->BVN3 gap=94 deliv=9210 undeliv=synthetic BVN2->BVN1 undeliv=4961 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 0 |
| wedge captures (#4125) | 1 wedge-20260824T093539Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
