# Soak runs

Every run appends one row. Details in `<runId>/manifest.md`.

| run | commit | executor | healing | elapsed | exit | dn height | heals | note |
|---|---|---|---|---|---|---|---|---|
| [20260819T192510Z](20260819T192510Z/manifest.md) | `10k-tps-358-g12cd7a723` | v2-kourou | unconditional (DI conductor, #4105) | 0.25h | 0 | 1932→32513 | 8→21 | local 3BVNx4val: verify reporting fixes — heals visible, anchor sent real, diagonal shown (#4075 #4093 #4095) |
| [20260819T203858Z](20260819T203858Z/manifest.md) | `10k-tps-359-g2ebce8422-dirty` | v2-kourou | unconditional (DI conductor, #4105) | ?h | 143 | 1783→1783 | 16→16 | local rerun: 3BVNx4val, 2 TPS, chaos |
| [20260819T205323Z](20260819T205323Z/manifest.md) | `10k-tps-360-g67b287682-dirty` | v2-kourou | unconditional (DI conductor, #4105) | 0.25h | 0 | 2023→27414 | 224→13117 | local 10 TPS with the end+1 recovery fix (67b287682) — cadence + heal-under-chaos |
| [20260819T215957Z](20260819T215957Z/manifest.md) | `10k-tps-361-gf7dac217f-dirty` | v2-kourou | unconditional (DI conductor, #4105) | 0.25h | 0 | 144→2885 | 0→0 | 10 TPS with 3s blocks (#4098 f7dac217f) — before/after vs the 21-blocks-per-sec run |
| [20260819T234054Z](20260819T234054Z/manifest.md) | `10k-tps-362-g3edf25bf8-dirty` | v2-kourou | unconditional (DI conductor, #4105) | 4.64h | 1 | 131→33564 | 0→6955 | 12h DagBFT integration soak at f7dac217f (3s blocks, tip of issue-4105-collection-proof-delivery): does cross-partition delivery/healing ever engage at 3s block cadence (#4111)? Zeros-audit issues #4110-#4114 filed against run 20260819T215957Z before this run. |
| [20260820T050616Z](20260820T050616Z/manifest.md) | `10k-tps-370-gc04de643d-dirty` | v2-kourou | unconditional (DI conductor, #4105) | 0.25h | 0 | 127→4830 | 0→16005 | Shakedown of the #4115 fix stack (23fdd5388..c04de643d): rcmgr limits+metrics, tracker-poisoning fix, heal circuit breaker, backpressure requeue, metrics listener on every node. Chaos disabled — verifying transport health and #4111 delivery behaviour at 3s blocks before the 12h chaos-free run. |
