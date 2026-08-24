# Soak run 20260824T112437Z

**Purpose:** 1s blocks + latency-judged channels (1dc8a008d) + resized leveldb (ed2ba278b): 700 tps. Expect ~7s settlement, channels showing caught up; watching RSS slope for the OOM verdict

| field | value |
|---|---|
| started (UTC) | 2026-08-24T11:24:37Z |
| commit | `1dc8a008d5ef78447962b6be4e49437ad969bc66` |
| describe | `10k-tps-507-g1dc8a008d-dirty` |
| branch | `4138-conductor-owns-recovery` |
| uncommitted files | 51 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 12h |
| target TPS | 700 |

Config as run is frozen in `config/`. Results appended below on exit.
