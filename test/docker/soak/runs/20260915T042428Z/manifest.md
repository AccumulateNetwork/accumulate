# Soak run 20260915T042428Z

**Purpose:** BlockchainDB main @fac421a = PR #85 (seal off the lock) with its review fixes + PR #89 (history key management). Last bcdb soak 20260903T202621Z ran dcce242, the seal PR first commit only, and held 500 tps with block production 0.38-0.46s for 12 min before chaos. New since then: the seal resumes after a failure and the auto-seal is split off the shard lock; history bloom filters held to a budget instead of freed on handoff (#86 17.8% of CPU in segment.bloomTest, #88 11,152 history walks per commit); a failed dynamic-layer read no longer reported as not-found; segment and block-set headers validated against their files. THIS RUN ANSWERS: does the filter budget move the history read cost the profiles blamed, and is the merged seal+read path stable for 12h at 500 tps chaos off. WATCH: stats.json HistorySegments / ActiveSegments / ResidentBloomBytes (new gauges, #87) per node; accumulate_dagbft_block_production_seconds; loadgen rate near 500; zero unexpected not-found errors; mem.csv heapAllocMiB flat after hour 1.

| field | value |
|---|---|
| started (UTC) | 2026-09-15T04:24:28Z |
| commit | `590884c1b16a71194c3553ef24a34ad706984fe2` |
| describe | `10k-tps-678-g590884c1b-dirty` |
| branch | `issue-4263-stored-intermediates` |
| uncommitted files | 284 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:f643a91018306e7e69cb29d93a064ffaa3d744acedd82399353c247939cc7514` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 12h |
| target TPS | 500 |
| storage |  (docker-network.yml) |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/` (soak.conf, the compose and network files). Results appended below on exit.

## Result (recorded by hand; the harness was killed at minute 8)

Not an acceptance result. The Directory stopped executing batches at
05:41:33Z, 70 minutes in, and never resumed: block 4114 failed to close on
every node at 05:40:36Z (`index 2754 is outside the segment [2755, 2756]`),
eight more groups failed the same way within a minute, and the execution
lag of 9 against a bound of 8 has held since. The BVNs ran on at 500 tps,
refusing nothing, until the run was torn down. The store's part of the
question -- the filter budget and the merged seal path -- was not reached.
Analysis and fixes: `capture-heldview-140038Z/README.md`, #4279.

Torn down by hand at 2026-09-15T16:37Z: harness processes killed, `docker compose -p disoak down -v`, network and volume removed. The loadgen had already stopped at its 12h target (~16:34Z).

mem.csv (1.01 MiB, over the 1 MiB blob limit) stays local; its reading is in the review section of capture-heldview-140038Z/README.md.
