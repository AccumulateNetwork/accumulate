# Soak run 20260822T071949Z

**Purpose:** VERIFICATION 2 for #4132 on 9adf63f96: the routing fix was inert last run because the signer was read from the legacy envelope.Signatures field, which is always empty — signatures travel as messages. Now read from messages. THE TEST: routeKey must be non-empty in TRACE-SUBMIT, timestamp-not-increasing rejections should go to zero, and phase A should report 100/100 sub-treasuries funded.

| field | value |
|---|---|
| started (UTC) | 2026-08-22T07:19:49Z |
| commit | `9adf63f96cb35a6bb56162dfd506cc7312991d16` |
| describe | `10k-tps-433-g9adf63f96-dirty` |
| branch | `issue-4105-collection-proof-delivery` |
| uncommitted files | 6 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 20m |
| target TPS | 10 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-22T07:41:46Z |
| elapsed | 0.0h |
| driver exit | 0 (clean) |
| dn height | 121 -> 8975 |
| heals | 0 -> 44 |
| chaos events | 12 |
| monitor samples | 53 |
| seizure | none detected |
| reconcile pulls (#4073) | 74 |
| stalled channels at end | 1 |
| wedge captures (#4125) | 2 wedge-20260822T072505Z,wedge-20260822T074009Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
