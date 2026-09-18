# Soak run 20260917T223150Z

**Purpose:** 24h at 100 tps, chaos on, one rate: DI 816fde076 with #4284 healing guard, #4283 accounting, #4282 byte budget, #4277 heartbeat+restart seed; #4287 known

| field | value |
|---|---|
| started (UTC) | 2026-09-17T22:31:50Z |
| commit | `816fde076ddeb6d226a63e5c77738801786d3735` |
| describe | `10k-tps-752-g816fde076-dirty` |
| branch | `dagbft-integration` |
| uncommitted files | 348 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:e2ef7ce0d236613e7112a3cca708fa93f0e6b3cb3c1624e5e79edc822c99cad2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| fault model | restart or pause one BVN container every 480s + 0-240s |
| topology | 3 BVNs, 12 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 BVN3 |
| chaos | on |
| target duration | 24h |
| target TPS | 100 |
| storage |  (docker-network.yml) |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/` (soak.conf + override.conf, the compose and network files). Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-09-18T00:43:48Z
- reason: stalled 245s: BVN1,BVN2,BVN3,Directory (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.
