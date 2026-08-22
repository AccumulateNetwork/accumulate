# Soak run 20260822T054945Z

**Purpose:** 12h chaos soak on f41ca9c0f. Consensus: #4125 retention window + re-delivered-certificate skip, #4128 committed batches stay fetchable. Loadgen: #4130 token accounts funded and balance awaited before advertising, #4129 lock-account skips while no major blocks exist. Harness: soakmon supervised and restarted if it dies (it died mid-run twice today, cause unexplained, now logs its own exit), stallkill stops a run whose monitor is unreachable 120s, both watchdogs ignore the idle bootstrap window. Watch: redelivered stays 0, batch waits none, tx/s toward 10, and soakmon.log for any restart line.

| field | value |
|---|---|
| started (UTC) | 2026-08-22T05:49:45Z |
| commit | `f41ca9c0f0375a8a5b9f7939af800f522d8fc826` |
| describe | `10k-tps-425-gf41ca9c0f` |
| branch | `issue-4105-collection-proof-delivery` |
| uncommitted files | 0  |
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

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-22T05:54:01Z |
| elapsed | ?h |
| driver exit | 143 (FAILED) |
| dn height | 121 -> 121 |
| heals | 0 -> 0 |
| chaos events | 0 |
| monitor samples | 1 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 0 |
| wedge captures (#4125) | 0  |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
