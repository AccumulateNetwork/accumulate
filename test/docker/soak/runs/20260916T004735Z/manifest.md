# Soak run 20260916T004735Z

**Purpose:** Verification of the COMPLETE #4280 fix on dagbft-integration de7e6323d: healing asks for nothing until a stream's Delivered has sat still for probeAfter (4) activations — holes included, because a stream executes in order so a moving Delivered proves nothing below it is missing. 1h, 500 tps, CHAOS OFF from soak.conf. Baselines, same conditions: 20260915T211229Z ~5,600 healed entries/min (no gate); 20260915T233806Z ~1,500/min (probe gated only). THIS RUN ANSWERS: does a fault-free network now heal essentially nothing? WATCH: conductor_heal_entries_total near zero and flat; answered near zero; not-yet collapsed; miss 0; every stream's delivered advancing; block production near 1s; no SEQUENCE REGRESSION. Also reads the exported hand-off accounting (#4279). NOTE: the previous attempt (20260916T001538Z) died in 44s because the prior run's loadgen still held 8091 — ports verified free before this launch.

| field | value |
|---|---|
| started (UTC) | 2026-09-16T00:47:35Z |
| commit | `de7e6323db397b612f587d75f41fdac059dc8839` |
| describe | `10k-tps-700-gde7e6323d-dirty` |
| branch | `dagbft-integration` |
| uncommitted files | 273 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:5f1d76cc0df3334ffce181a93a50518c19d3215c33a69e777da9bfe37f764aef` |
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

## Stopped early (~20 min of 1h): the probe wait was inside the delivery path

The complete #4280 gate (de7e6323d) took a clean network from ~5,600 healed
entries/min (no gate) to ~1,500 (probe gated) to ~304/min here — and the
hand-off accounting read perfectly: 2,246,174 arrived, 2,246,174 executed,
nothing unaccounted, no dropped envelopes, no heal misses.

The remaining ~304/min was the probe waking inside normal delivery. A block's
synthetics wait for a Directory receipt before they leave, so the path is the
proof-path latency; measured here, synthetic streams ran 13.7-29.3s in flight
at 500 tps. probeAfter=4 fires at sixteen blocks, inside that, so the source
answered from its cache with what it had produced and not yet dispatched and
healing delivered what dispatch was about to. Raised to 8 (thirty-two blocks),
clear of the measured path.

Also seen: execution lag 9-13 blocks against a bound of 8, mildly throttling
accepted load to ~485 tps. And the anchor rows of the flow matrix reported
NEGATIVE in-flight latency (about -1,200s), which is an impossible state and
wants its own look.
