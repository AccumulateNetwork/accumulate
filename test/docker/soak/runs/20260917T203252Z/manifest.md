# Soak run 20260917T203252Z

**Purpose:** 24h at 100 tps, chaos on, one rate throughout: #4277 heartbeat, #4277 restart anchor-serving, #4282 byte-bounded anchor staging

| field | value |
|---|---|
| started (UTC) | 2026-09-17T20:32:52Z |
| commit | `aa2e63b2ebfb7615812a7ddf84cef37c24873e16` |
| describe | `10k-tps-747-gaa2e63b2e` |
| branch | `dagbft-integration` |
| uncommitted files | 325 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:5aef17105c8181dd2b847029d95ac98d13b4fb9403ce59f2406dbd64647dabf2` |
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

## Ended

Stopped by hand at ~21:35Z, about an hour in, once the cause was understood.
Delivery into BVN3 stopped at 20:43 when chaos paused acc-bvn3-val2: two
one-entry holes (BVN1 #1600, BVN2 #5457) that the requester never asked for,
because it refuses at any execution lag above zero and a working node is one
behind at the hook where healing runs. Filed as #4284 (the guard), #4286
(the loss), #4285 (the watchdogs that cannot see this). Evidence in
`stall-capture/`. The #4282 byte budget held: staged-proof refusals stayed at
zero throughout.
