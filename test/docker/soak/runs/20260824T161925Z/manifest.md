# Soak run 20260824T161925Z

**Purpose:** 12h pass attempt #6 on 345a54146: 3s blocks (the proven interval) + every fix of the last 24h - block-per-commit, byte caps, DAG 2k, GOMEMLIMIT, 256MB caches, capped universe, QUERY GATES, record cache. 250 tps, maximum margin

| field | value |
|---|---|
| started (UTC) | 2026-08-24T16:19:25Z |
| commit | `345a54146ad2838bfdfd7d46a83ccad42ffa138e` |
| describe | `10k-tps-516-g345a54146-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 59 (see config/uncommitted.patch) |
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

- stopped (UTC): 2026-08-24T17:06:10Z
- reason: stalled 244s: BVN1,BVN2,Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.
