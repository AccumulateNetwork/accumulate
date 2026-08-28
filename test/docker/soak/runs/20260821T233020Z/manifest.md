# Soak run 20260821T233020Z

**Purpose:** 12h chaos soak at the tip of issue-4105-collection-proof-delivery (d81c52120): first full-length run since the test campaign (#4117-#4119, #4122 consumer fix, #4123 stalled-partition reporting, #4124 compose pin). Carries the #4125 answer — pprof on every node and wedgewatch dumping goroutines the moment a partition stalls, the artifact the 08-21 wedge was torn down without.

| field | value |
|---|---|
| started (UTC) | 2026-08-21T23:30:20Z |
| commit | `d81c5212091acd61ae1ee79bc8087db3ed9fc3de` |
| describe | `10k-tps-410-gd81c52120-dirty` |
| branch | `issue-4105-collection-proof-delivery` |
| uncommitted files | 1 (see config/uncommitted.patch) |
| image | `docker-bvn1-val1` |
| image id | `sha256:65b858c9077fda33b4a36f055f914a456779e8a589508f12ad25459065abb3f2` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 12h |
| target TPS | 10 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-08-22T00:40:55Z |
| elapsed | 1.06h |
| driver exit | 143 (FAILED) |
| dn height | 121 -> 2952 |
| heals | 0 -> 38 |
| chaos events | 6 |
| monitor samples | 14 |
| seizure | SEIZED at 2026-08-21T23:51:37 :: stalled stream, undelivered for 20 polls :: stuck=0 stuckStream= worst=BVN2->Directory gap=0 deliv=8 undeliv=anchor BVN2->Directory undeliv=1622 |
| reconcile pulls (#4073) | 14126 |
| stalled channels at end | 8 |
| wedge captures (#4125) | 3 wedge-20260821T233446Z,wedge-20260821T234951Z wedge-20260822T000455Z |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.

## Why this run was stopped at ~1.2h of a 12h target

The Directory halted permanently at height 2952 from 23:37:39 and could not
recover, so the remaining 11 hours would have measured a broken network — every
cross-partition anchor depends on the DN. The loadgen was signalled so the
script could write its own verdict, then the network was torn down.

The run did its job: it reproduced #4125 within 90 seconds of load and captured
it three times, which is what the pprof-on-every-node and wedgewatch changes
were added for.

**Finding.** A committed certificate references a batch that no node in the
network has. `CollectBatches` (consensus.go:286) retries forever and peer fetch
cannot rescue it, because every validator is stuck on the same round needing the
same batch. 190,500 `Waiting for batches ... missing=1 partition=Directory
round=246` warnings, one round, one batch, every node. All four partitions are
susceptible — Directory 246, BVN3 984, BVN2 1416/1650, BVN1 1.

**Ruled out:** LRU eviction. It fires 5,558 times but the first is at 00:04:43,
27 minutes after the wedge — a consequence of the backlog, not a cause.

**Not established:** how the batch goes missing. See #4125 for the candidate
worth testing next and the instrumentation needed to settle it.

Captures: `wedge-20260821T233446Z` (all four partitions, the 280s execution
stall that self-recovered), `wedge-20260821T234951Z` (DN wedged, the decisive
`CollectBatches` stack), `wedge-20260822T000455Z` (spread to BVN1/BVN3).
