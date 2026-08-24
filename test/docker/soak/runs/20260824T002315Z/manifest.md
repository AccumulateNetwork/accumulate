# Soak run 20260824T002315Z

**Purpose:** validate #4159 REAL fix (data-availability enforced at vote time, 5e335dfb7) + fixed watchdogs; must run clean well past DN 553 and 4152; branch 4138-conductor-owns-recovery @ 5e335dfb7

| field | value |
|---|---|
| started (UTC) | 2026-08-24T00:23:15Z |
| commit | `5e335dfb7e5a1e623fb5945d7d62429f73151cb3` |
| describe | `10k-tps-481-g5e335dfb7-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 6 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 12h |
| target TPS | 2 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-24T01:52:47Z |
| elapsed | ?h |
| driver exit | 143 (FAILED) |
| dn height | 121 -> 529 |
| heals | 0 -> 0 |
| chaos events | 8 |
| monitor samples | 18 |
| seizure | none detected |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 0 |
| wedge captures (#4125) | 2 wedge-20260824T015054Z,wedge-manual-529 |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.

## Stopped early by stallkill

- stopped (UTC): 2026-08-24T01:54:45Z
- reason: monitor unreachable for 120s

Evidence was captured before stopping; see the probe-* directory
written at that moment.
