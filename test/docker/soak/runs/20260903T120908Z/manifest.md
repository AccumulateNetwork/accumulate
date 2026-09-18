# Soak run 20260903T120908Z

**Purpose:** bcdb 12h/500tps on 5294c6e28 -- the first build where the primary does not deadlock, so the first whose result means anything. The two runs before this (20260903T035139Z, 20260903T042628Z) wedged BVN2 at 23 and 8 minutes because cleanupOldHeaders held pendingMu and my pin-release re-entered it: a deadlocked primary stops advancing rounds while gossip flows, peers stay connected and containers report healthy, which is why chasing the synthetic stream never converged. consim's TestSkewedLoad catches it in 17s (hangs past 150s on the broken commit) and now counts submit refusals, which it previously discarded. Carries: batch pin with the lifecycle tested end-to-end through OnHeaderReceived, submit no longer refuses work (crossing a pending boundary seals synchronously and keeps the envelope), #4189 durable unhashed staging, #4201 heal cadence, two bcdb read caches, tally reduced to counters, 2048m/1700MiB containers. WATCH: BVN2 block production must not stop; recv-deliv must never pin at 4096; BVN1 received-from-BVN2 must track BVN2 produced-for-BVN1.

| field | value |
|---|---|
| started (UTC) | 2026-09-03T12:09:09Z |
| commit | `5294c6e282ae354150a61117e266d038593167bc` |
| describe | `10k-tps-664-g5294c6e28-dirty` |
| branch | `issue-4189-staging-out-of-account` |
| uncommitted files | 9 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:8223d8b6347e4a9c2b7d2a8a946175c440cfd4ea3f448cd98b2690027dab7161` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | on |
| target duration | 12h |
| target TPS | 500 |
| storage | BlockchainDB |
| memory budget | mem_limit 1536m, GOMEMLIMIT 1200MiB |

Config as run is frozen in `config/`. Results appended below on exit.
