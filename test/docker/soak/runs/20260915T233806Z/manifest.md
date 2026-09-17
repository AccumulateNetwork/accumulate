# Soak run 20260915T233806Z

**Purpose:** Verification of the #4280 heal-storm fix on dagbft-integration f1e852eac: the catch-up probe now waits for a stream to be empty AND still for probeAfter (4) activations before asking for the span above Delivered. 1h, 500 tps, CHAOS OFF (from soak.conf, which owns the knob) — a fault-free network, the same conditions that produced 743,000 healed entries against 23,000 requests before the fix. THIS RUN ANSWERS: does a clean network now heal ~nothing? WATCH: conductor_heal_entries_total and heal requests answered should stay near zero; not-yet should fall sharply; miss should be 0; no SEQUENCE REGRESSION lines; every stream's delivered advancing; block production near 1s. Also carries the exported hand-off accounting (#4279), so the Not executed card reads for the first time: arrived minus executed, and what the failure outcomes do not account for.

| field | value |
|---|---|
| started (UTC) | 2026-09-15T23:38:06Z |
| commit | `f1e852eac315a985dda58e7b61c6e75d088fad5d` |
| describe | `10k-tps-699-gf1e852eac-dirty` |
| branch | `dagbft-integration` |
| uncommitted files | 270 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:32e0cbf0fddb42ed09ea6749fd9d12928c05c6fde0f0c5d8933a0370c0cd0d07` |
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

## Stopped early (~26 min of 1h): the fix was incomplete

Healing fell from ~5,600 entries/min to ~1,500, so the gated catch-up probe
worked, but a clean network was still healing. The remainder came from the
hole walk in `decide`: answers averaged 49 consecutive numbers — a package in
flight, not a loss. Stopped to finish the fix (de7e6323d): healing now asks
for nothing until Delivered has sat still, holes included, because a stream
executes in order and a moving Delivered proves nothing below it is missing.
