# Soak run 20260824T145858Z

**Purpose:** 12h pass attempt #5 on b6a510f6e: QUERY GATES (the DoS boundary) + write-path record cache + everything prior. 2s blocks, 250 tps - the rate that collapsed at t+31 without these. If the gates hold, pollers can no longer amplify lag into collapse

| field | value |
|---|---|
| started (UTC) | 2026-08-24T14:58:58Z |
| commit | `b6a510f6e2d30b264870367d010962994a445ae5` |
| describe | `10k-tps-515-gb6a510f6e-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 58 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 12h |
| target TPS | 250 |

Config as run is frozen in `config/`. Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-08-24T15:40:23Z
- reason: stalled 244s: BVN1,BVN2 (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-24T15:40:23Z |
| elapsed | 0.61h |
| driver exit | 143 (FAILED) |
| dn height | 20 -> 1074 |
| heals | 24 -> 65917 |
| chaos events | 2 |
| monitor samples | 8 |
| seizure | SEIZED at 2026-08-24T15:18:12 :: stalled stream, undelivered for 20 polls :: stuck=0 stuckStream= worst=BVN1->Directory gap=0 deliv=389 undeliv=synthetic BVN2->BVN1 undeliv=874 |
| reconcile pulls (#4073) | 8324 |
| stalled channels at end | 0 |
| wedge captures (#4125) | 1 wedge-20260824T153108Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
