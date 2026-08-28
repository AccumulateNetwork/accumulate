# Soak run 20260822T045407Z

**Purpose:** 12h chaos soak on the #4125/#4128 fixes (e54e4b895): committed batches now enter a 10m/4096 retention window instead of being deleted on commit, and a certificate whose missing batch its own commit retired is skipped as a re-delivery instead of waited on forever. Previous run halted the Directory in 8 minutes on round 260 and stranded 3 of 12 validators after restart/pause; this run should survive both. stallkill armed at 60s.

| field | value |
|---|---|
| started (UTC) | 2026-08-22T04:54:07Z |
| commit | `e54e4b89598d1a219fa88793957327e9d769304e` |
| describe | `10k-tps-417-ge54e4b895` |
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

## Stopped early by stallkill

- stopped (UTC): 2026-08-22T04:57:52Z
- reason: stalled 65s: BVN1,BVN2,BVN3,Directory (threshold 60s)

The run was ended once a partition had been stalled past the
threshold. Evidence was captured before stopping; see the probe-*
directory written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-22T04:57:52Z |
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
