# Soak run 20260819T203858Z

**Purpose:** local rerun: 3BVNx4val, 2 TPS, chaos

| field | value |
|---|---|
| started (UTC) | 2026-08-19T20:38:58Z |
| commit | `2ebce8422a3aba83b0897b32ae4f38d135146925` |
| describe | `10k-tps-359-g2ebce8422-dirty` |
| branch | `issue-4105-collection-proof-delivery` |
| uncommitted files | 5 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:a506d9376ef6695a5e42a7836eb0fce3664dc54ad20b0efeae6220d3a2c9837f` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 20m |
| target TPS | 2 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-19T20:40:42Z |
| elapsed | ?h |
| driver exit | 143 (FAILED) |
| dn height | 1783 -> 1783 |
| heals | 16 -> 16 |
| chaos events | 0 |
| monitor samples | 1 |
| seizure | none detected |
| reconcile pulls (#4073) | 19 |
| stalled channels at end | 2 |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
