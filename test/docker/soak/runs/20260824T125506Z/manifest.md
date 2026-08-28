# Soak run 20260824T125506Z

**Purpose:** 12h pass attempt #2 on 5283060ca: GOMEMLIMIT 2GiB (1250MiB throttled the GC into a collapse), DAG GC 2k, byte caps, 2s blocks, 400 tps

| field | value |
|---|---|
| started (UTC) | 2026-08-24T12:55:06Z |
| commit | `5283060caceb3a0dfc3b03dc51c113631d86ecca` |
| describe | `10k-tps-512-g5283060ca-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 55 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 12h |
| target TPS | 400 |

Config as run is frozen in `config/`. Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-08-24T13:19:34Z
- reason: stalled 249s: BVN2,Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-24T13:19:34Z |
| elapsed | 0.34h |
| driver exit | 143 (FAILED) |
| dn height | 20 -> 503 |
| heals | 24 -> 53913 |
| chaos events | 2 |
| monitor samples | 5 |
| seizure | SEIZED at 2026-08-24T13:09:12 :: stuck=0 stuckStream= worst=BVN2->BVN1 gap=233 deliv=51361 undeliv=synthetic BVN2->BVN1 undeliv=4243 |
| reconcile pulls (#4073) | 2163 |
| stalled channels at end | 0 |
| wedge captures (#4125) | 1 wedge-20260824T131725Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
