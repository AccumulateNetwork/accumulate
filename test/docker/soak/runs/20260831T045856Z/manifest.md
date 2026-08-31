# Soak run 20260831T045856Z

**Purpose:** SOAK-12H-500-SM: 12h CHAOS soak at 500 tps on BlockchainDB 12fe469 (fix/streaming-merge: promote-only seal #60, streaming merge #59, unlink audit #61) + adapter background maintenance (a864f45cf), N=64 at creation. First soak with chaos ON for this backend — chaos restarts validators, so this is also the first live test of the #4173 restart fixes (version resumes from the manifest, persisted exception set). Prior: eea0b86 paused 11-29 s in sealDynaIfFull->rewriteLiveFile (goroutine dump in 20260830T172800Z/stall-dumps). Acceptance: zero Partition-stalled lines from the auto-seal, blocks 3.0 s throughout, restarted nodes rejoin and commit (no seal-height errors), probe max flat, maintenanceErrors 0, no misroutes/conflicts, heals return to zero after each chaos event.

| field | value |
|---|---|
| started (UTC) | 2026-08-31T04:58:57Z |
| commit | `6a980cd4992912e2c5676af664241f501b2d7c1c` |
| describe | `10k-tps-608-g6a980cd49` |
| branch | `bcdb-storage-backend` |
| uncommitted files | 3 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:88d0f7ad636e9111901b2fb278cf7c3c2a67b24c5dddd3314e4aa3fa7ea1f76e` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | on |
| target duration | 12h |
| target TPS | 500 |
| storage | blockchainDB |
| memory budget | mem_limit 2560m, GOMEMLIMIT 2GiB |

Config as run is frozen in `config/`. Results appended below on exit.

## Result (stopped by Paul at 05:16Z, 17 min)

No chaos event had fired (the first slot rolled a skip). Network healthy
throughout: 3.0 s blocks, heals 0, 499.4 tx/s achieved, 1 rejected.

## Reading

The auto-seal pauses of 20260830T172800Z did not appear — zero
Partition-stalled lines in 17 minutes on 12fe469 (promote-only seal, #60).

**CPU climbed instead: history lookups probe per-segment bloom filters.**
By 05:12 every node was at 140–160 % and rising while blocks held 3.0 s.
The profile (profiles/0512-bvn2-val1-cpu.pb.gz): reads are 27 % of CPU,
`SegmentStore.lookupHistory` 23 %, `Bloom.Test` 11 % flat — 4 s of a 30 s
sample in bloom probes. #56 rolled the key filter to cover only the newest
128 blocks, so a lookup that falls through to history probes each history
segment's own filter: cost ∝ history segment count, growing per block until
the streaming merge (#59) collapses segments. The next store question is
whether merged history keeps a single filter (or an index) so a miss is
O(tiers), not O(segments).

Store correctness held: 0 conflicts, 0 misroutes, maintenanceErrors 0.
Paul's local-tree `replace` was parked in a stash for the run and restored.
