# Soak run 20260916T012820Z

**Purpose:** Verification of #4280 with the probe wait clear of the delivery path (dagbft-integration 3013d4ad0, probeAfter=8 = 32 blocks). 1h, 500 tps, CHAOS OFF from soak.conf. Baselines on identical conditions: 20260915T211229Z ~5,600 healed entries/min (ungated); 20260915T233806Z ~1,500 (probe gated only); 20260916T004735Z ~304 (gate complete, probeAfter=4, firing inside the 13.7-29.3s delivery path). THIS RUN ANSWERS: with the wait beyond the measured path, does a fault-free network heal essentially nothing? WATCH: conductor_heal_entries_total near zero and FLAT; answered ~0; miss 0; hand-off arrived==executed with nothing unaccounted; every stream's delivered advancing; no SEQUENCE REGRESSION. Known separately: execution lag sits 9-13 blocks against a bound of 8, mildly throttling load; the anchor rows report negative in-flight latency, an impossible state wanting its own fix.

| field | value |
|---|---|
| started (UTC) | 2026-09-16T01:28:20Z |
| commit | `3013d4ad0eb4e782087918c39d3f39f01da126f5` |
| describe | `10k-tps-701-g3013d4ad0-dirty` |
| branch | `dagbft-integration` |
| uncommitted files | 274 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:acc76c4d2260e6b24253b85d09f46f8726f9d5ed3cdf254ea4ccaa2a2146e11e` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 1h |
| target TPS | 500 |
| storage |  (docker-network.yml) |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/` (soak.conf, the compose and network files). Results appended below on exit.

## Stopped by hand (~25 min of 1h)

The question it was asked was answered well before the hour. With probeAfter=8
(thirty-two blocks, clear of the measured 13.7-29.3s delivery path), a
fault-free network healed ~72 entries/min against these baselines on identical
conditions:

| run | gate | healed entries/min |
|---|---|---|
| 20260915T211229Z | none | ~5,600 |
| 20260915T233806Z | probe only | ~1,500 |
| 20260916T004735Z | complete, probeAfter=4 | ~304 |
| this run | complete, probeAfter=8 | ~72 |

Clean beside it: 2,889,176 transactions arrived at execution and 2,889,176
executed, nothing unaccounted; no dropped envelopes; zero heal misses; all
three partitions at 1.00 s/block throughout.

What it also measured, with the heal storm removed: the executor still cannot
hold 500 tps. BVN2 ran 17-20 blocks behind on all four validators and BVN1
8-10, so the lag bound refused submissions and accepted load settled at ~462.
That is #4258's wall, now measured without healing noise on top — so the
amplification was aggravating the ceiling, not causing it.
