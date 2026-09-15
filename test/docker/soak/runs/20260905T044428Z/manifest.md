# Soak run 20260905T044428Z

**Purpose:** ACCEPTANCE RUN #7 (second start), 12 H, CHAOS OFF, on issue-4193-producer-cache @f8e4bbc50 = run 20260905T032333Z (H1, D7, D8, R3, E10) plus E9 (single-site append), C6 (execution lag bound 8 blocks), H8 (healing requester pulls spans from staging; the source serves only blocks dispatched 8+ blocks ago and answers the dispatched prefix; not-yet is not a failure), DN height from the ledger index. First start 20260905T042031Z stopped at 15 min: the requester asked for undispatched tails. THIS RUN ANSWERS: heals == 0 and no answered heal requests with nothing dropped (not-yet outcomes allowed); does C6 stop the BVN2 GC spiral (#4220): BVN2 block rate vs BVN1, execution_lag_blocks, batch_store_refusing{execution-lagging}; does memory plateau after the cache horizon (1h); shallowMisses without MainChain.ElementIndex (E9); Directory store history reads ~0. 12 hours: long enough for a claim.

| field | value |
|---|---|
| started (UTC) | 2026-09-05T04:44:28Z |
| commit | `f8e4bbc50a7cc78fdf801052296146eaaeb42f45` |
| describe | `10k-tps-747-gf8e4bbc50-dirty` |
| branch | `issue-4193-producer-cache` |
| uncommitted files | 179 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:e6e694b9442297b85f0874da6b288b8214dda70838c6854e1c2d7f9df1e2f7c3` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 12h |
| target TPS | 500 |
| storage | BlockchainDB |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/`. Results appended below on exit.

## Stopped at 25 minutes — not by the network

The shell that launched this run was stopped at 05:09Z along with every other
background task of the session; soak.sh, the load generator and the monitor
died with it and the containers were left running unobserved. Torn down by
hand at 05:10Z and relaunched detached (setsid) as the next run. No finding
from this run; heals were 0 → 0 and the DN reached height 1298 in 25 minutes.
