# Soak run 20260824T014438Z

**Purpose:** validate the #4159 batch-recovery fixes on real nodes (partition-scoped fetch protocol, fetch-on-deferral, rebroadcast-on-repropose, CanServeBatch; consim: 8/8 passes vs 1/6 baseline); must run far past DN 553; branch 4138-conductor-owns-recovery @ b85031273

| field | value |
|---|---|
| started (UTC) | 2026-08-24T01:44:38Z |
| commit | `b85031273a90367e929f69b993fcc2f22f1de665` |
| describe | `10k-tps-486-gb85031273-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 11 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 12h |
| target TPS | 2 |

Config as run is frozen in `config/`. Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-08-24T01:52:47Z
- reason: stalled 240s: BVN1,BVN2,BVN3,Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-24T01:52:47Z |
| elapsed | ?h |
| driver exit | 143 (FAILED) |
| dn height | 121 -> 121 |
| heals | 0 -> 0 |
| chaos events | 0 |
| monitor samples | 1 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 0 |
| wedge captures (#4125) | 1 wedge-20260824T015054Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
