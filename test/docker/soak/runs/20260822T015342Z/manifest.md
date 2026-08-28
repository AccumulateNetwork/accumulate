# Soak run 20260822T015342Z

**Purpose:** 12h chaos soak on the instrumented build (615cacf24, #4125): batch tombstones + rate-limited CollectBatches diagnostics. The 20260821T233020Z run halted the Directory permanently on one missing batch of the round-246 certificate and could not say why; this run should name the cause — pruned-after-commit, evicted-lru, or never stored — on the first stalled round. Also first run with the #4126 provenance fixes.

| field | value |
|---|---|
| started (UTC) | 2026-08-22T01:53:42Z |
| commit | `df4c4f98d42528549cdb98f19b1ca6b014716cbb` |
| describe | `10k-tps-414-gdf4c4f98d-dirty` |
| branch | `issue-4105-collection-proof-delivery` |
| uncommitted files | 1 (see config/uncommitted.patch) |
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

## Stopped early by stallkill

- stopped (UTC): 2026-08-22T04:26:04Z
- reason: stalled 8673s: BVN1,Directory (threshold 60s)

The run was ended once a partition had been stalled past the
threshold. Evidence was captured before stopping; see the probe-*
directory written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-22T04:26:04Z |
| elapsed | 2.42h |
| driver exit | 143 (FAILED) |
| dn height | 121 -> 3117 |
| heals | 0 -> 46 |
| chaos events | 13 |
| monitor samples | 30 |
| seizure | SEIZED at 2026-08-22T02:15:03 :: stalled stream, undelivered for 20 polls :: stuck=0 stuckStream= worst=BVN1->Directory gap=0 deliv=5 undeliv=anchor BVN1->Directory undeliv=1302 |
| reconcile pulls (#4073) | 21811 |
| stalled channels at end | 9 |
| wedge captures (#4125) | 3 wedge-20260822T015813Z,wedge-20260822T021317Z wedge-20260822T022822Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.
