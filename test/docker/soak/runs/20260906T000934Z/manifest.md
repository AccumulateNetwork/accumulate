# Soak run 20260906T000934Z

**Purpose:** MEMORY PROFILE, 7 MIN, CHAOS OFF: run 235425Z delivered every transaction but BVN2 RSS went 103 -> 1300 MiB in seven minutes (pre-E12 eighth start: 76 -> 741 in the same window). The in-process measurement (20k transfers on BlockchainDB) retains 80 MiB and shows the store, not the executor. THIS RUN ANSWERS: what holds a Docker node's heap under 500 tps — heap profiles are pulled from bvn2-val1 and bvn1-val1 at minutes 4.5 and 6 of load. A diagnostic, not a soak.

| field | value |
|---|---|
| started (UTC) | 2026-09-06T00:09:34Z |
| commit | `0d537c46b5c878c57be467c620a47bbd6bbf40c4` |
| describe | `10k-tps-770-g0d537c46b-dirty` |
| branch | `issue-4193-producer-cache` |
| uncommitted files | 288 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:02c2a1dd5a56d4f72509386f9f72d5510469a6f4b0679708ab76158ffc69012d` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 7m |
| target TPS | 500 |
| storage | leveldb |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/`. Results appended below on exit.
