# Soak run 20260822T062523Z

**Purpose:** DIAGNOSTIC run for #4132, not a soak: 20m with ACC_TX_TRACE=1 to follow every transaction from acceptance through batching to execution, and per-block execution accounting (arrived vs executed vs unmarshalFailed). Question: of the 100 bootstrap deposits, do the ~95 that never execute appear in a batch? If yes the loss is in execution; if no it is in consensus. stallkill disabled — the network is EXPECTED to sit idle at DN 553.

| field | value |
|---|---|
| started (UTC) | 2026-08-22T06:25:23Z |
| commit | `129b293e8af56f864a4fc46cc27141dbf469579f` |
| describe | `10k-tps-429-g129b293e8-dirty` |
| branch | `issue-4105-collection-proof-delivery` |
| uncommitted files | 3 (see config/uncommitted.patch) |
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
