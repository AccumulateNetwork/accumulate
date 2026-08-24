# Soak run 20260824T021103Z

**Purpose:** validate #4159 stall-3 fix (lastAuthoredRound watermark kills self-equivocation) on real nodes; batch fixes already verified in prior run; consim 6/6; branch @ 2f36fe66b

| field | value |
|---|---|
| started (UTC) | 2026-08-24T02:11:03Z |
| commit | `2f36fe66b8eed814825c2d3a44d9351fe933637d` |
| describe | `10k-tps-487-g2f36fe66b-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 35 (see config/uncommitted.patch) |
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

- stopped (UTC): 2026-08-24T02:18:25Z
- reason: stalled 243s: BVN1,BVN2,BVN3,Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-24T02:18:25Z |
| elapsed | ?h |
| driver exit | 143 (FAILED) |
| dn height | 1 -> 1 |
| heals | 0 -> 0 |
| chaos events | 0 |
| monitor samples | 1 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 0 |
| wedge captures (#4125) | 1 wedge-20260824T021622Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
