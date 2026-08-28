# Soak run 20260822T063539Z

**Purpose:** DIAGNOSTIC 2 for #4132: signature timestamp rejections are now logged at their source. Question: does the bootstrap burst of 100 produce ~92 BadTimestamp rejections against the treasury key? That closes the chain from 100 submitted to 8 signatures recorded. num-workers is 100 on this network (default is 1, netsim uses 4) which is what scatters a signers transactions across workers and destroys their execution order.

| field | value |
|---|---|
| started (UTC) | 2026-08-22T06:35:39Z |
| commit | `a5e002cdeb41756235b50b96d71e904bbba2010b` |
| describe | `10k-tps-430-ga5e002cde-dirty` |
| branch | `issue-4105-collection-proof-delivery` |
| uncommitted files | 4 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 15m |
| target TPS | 10 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-22T06:57:30Z |
| elapsed | 0.0h |
| driver exit | 0 (clean) |
| dn height | 121 -> 6016 |
| heals | 0 -> 81 |
| chaos events | 10 |
| monitor samples | 53 |
| seizure | none detected |
| reconcile pulls (#4073) | 88 |
| stalled channels at end | 2 |
| wedge captures (#4125) | 2 wedge-20260822T064030Z,wedge-20260822T065535Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
