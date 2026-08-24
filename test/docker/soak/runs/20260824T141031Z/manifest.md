# Soak run 20260824T141031Z

**Purpose:** 12h pass attempt #4 on 363a3b3b0: state-scaling package (256MB caches, 25k account cap, GOMEMLIMIT 2750MiB) + 2s blocks, 250 tps. The state-size death clock is now bounded: working set fits the cache by construction

| field | value |
|---|---|
| started (UTC) | 2026-08-24T14:10:31Z |
| commit | `363a3b3b06135ee4a74bfd329731b7476b3ee7e7` |
| describe | `10k-tps-514-g363a3b3b0-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 57 (see config/uncommitted.patch) |
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

- stopped (UTC): 2026-08-24T14:57:41Z
- reason: stalled 245s: BVN1,BVN2,Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-24T14:57:41Z |
| elapsed | 0.71h |
| driver exit | 143 (FAILED) |
| dn height | 20 -> 1121 |
| heals | 24 -> 68576 |
| chaos events | 2 |
| monitor samples | 9 |
| seizure | SEIZED at 2026-08-24T14:29:27 :: stalled stream, undelivered for 20 polls :: stuck=0 stuckStream= worst=BVN1->Directory gap=0 deliv=376 undeliv=synthetic BVN2->BVN1 undeliv=911 |
| reconcile pulls (#4073) | 3327 |
| stalled channels at end | 0 |
| wedge captures (#4125) | 1 wedge-20260824T145535Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
