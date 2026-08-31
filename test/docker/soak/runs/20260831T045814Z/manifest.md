# Soak run 20260831T045814Z

**Purpose:** RUN-4H-500-SM: 4h at 500 tps on BlockchainDB 12fe469 (fix/streaming-merge: promote-only seal for #60, streaming merge #59, unlink audit #61) + adapter background maintenance (a864f45cf), N=64 at creation. The eea0b86 run 20260830T172800Z paused 11-29 s every 4-5 blocks in sealDynaIfFull->rewriteLiveFile (pread per record under the store lock, goroutine dump in its stall-dumps/). Watch: zero Partition-stalled lines is the acceptance bar; blocks 3.0 s past 1000; probe max flat; maintenanceErrors 0; dyna live.dat and layer sizes; disk growth.

| field | value |
|---|---|
| started (UTC) | 2026-08-31T04:58:14Z |
| commit | `6a980cd4992912e2c5676af664241f501b2d7c1c` |
| describe | `10k-tps-608-g6a980cd49` |
| branch | `bcdb-storage-backend` |
| uncommitted files | 2 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:88d0f7ad636e9111901b2fb278cf7c3c2a67b24c5dddd3314e4aa3fa7ea1f76e` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 4h |
| target TPS | 500 |
| storage | blockchainDB |
| memory budget | mem_limit 2560m, GOMEMLIMIT 2GiB |

Config as run is frozen in `config/`. Results appended below on exit.
superseded immediately: relaunched as a 12h chaos soak (Paul: 'a soak test, if that wasn't clear'); nothing ran
