# Soak run 20260824T190257Z

**Purpose:** 12h pass attempt #8 on 5d5fa0560: buffer pool OFF for good (887MB regression found in attempt-7 post-mortem), honest cache accounting, full stack. 60 tps - the mega-channel at ~35/s, demonstrably sustainable. Overnight pass run; out-of-order synthetics is the next work item

| field | value |
|---|---|
| started (UTC) | 2026-08-24T19:02:57Z |
| commit | `5d5fa0560656e024ee89f69281118f3cc805231d` |
| describe | `10k-tps-517-g5d5fa0560-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 61 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 12h |
| target TPS | 60 |

Config as run is frozen in `config/`. Results appended below on exit.
