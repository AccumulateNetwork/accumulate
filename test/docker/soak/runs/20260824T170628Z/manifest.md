# Soak run 20260824T170628Z

**Purpose:** 12h pass attempt #7 on 345a54146: 100 tps - the rate with rock-solid evidence (84tps ran at 8% CPU). Full fix stack. The goal is A PASS; the frontier bisection happens offline afterward, not by burning live runs

| field | value |
|---|---|
| started (UTC) | 2026-08-24T17:06:28Z |
| commit | `345a54146ad2838bfdfd7d46a83ccad42ffa138e` |
| describe | `10k-tps-516-g345a54146-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 60 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 12h |
| target TPS | 100 |

Config as run is frozen in `config/`. Results appended below on exit.
