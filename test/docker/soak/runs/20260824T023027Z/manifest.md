# Soak run 20260824T023027Z

**Purpose:** validate #4159 stall-3 fix v2 (watermark with round-0 genesis exception, 98bfcfb5f); prior attempt wedged at round 0 from the v1 watermark — caught by liveness tripwires in 4 min

| field | value |
|---|---|
| started (UTC) | 2026-08-24T02:30:27Z |
| commit | `98bfcfb5f568c6391806ab41d2c68a5c19789062` |
| describe | `10k-tps-488-g98bfcfb5f-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 36 (see config/uncommitted.patch) |
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

- stopped (UTC): 2026-08-24T02:40:48Z
- reason: stalled 380s: BVN1,BVN3,Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-24T02:40:48Z |
| elapsed | ?h |
| driver exit | 143 (FAILED) |
| dn height | 121 -> 553 |
| heals | 0 -> 0 |
| chaos events | 0 |
| monitor samples | 2 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 0 |
| wedge captures (#4125) | 1 wedge-20260824T023635Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
