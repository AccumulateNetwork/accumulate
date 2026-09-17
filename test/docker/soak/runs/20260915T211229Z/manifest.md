# Soak run 20260915T211229Z

**Purpose:** Healing under chaos on the merged DI line (48a5614f9): first run of #4263-on-staging + #4279 together with DI's #4267 block interval, #4272 BPT anchoring and #4275 proof service. THIS RUN ANSWERS: does receiver-pull healing close the gaps chaos opens -- watch the healing panel's requests answered vs not-yet vs miss, entries healed, and what staging holds per stream (all newly wired, #4279; the old crosschain_* counters the board read have not existed for weeks). Also: streams now log Stream position/produced per block, so streamlog.py can read a stall out of the node log; sent/delivered are read from every node and a value that goes backwards is an alarm. WATCH: no SEQUENCE REGRESSION lines; heal requests answered climbing with miss/failed near zero; every stream's delivered advancing; block production near 1s.

| field | value |
|---|---|
| started (UTC) | 2026-09-15T21:12:29Z |
| commit | `48a5614f954a1b162329433e8ebf1619b86dd62b` |
| describe | `10k-tps-695-g48a5614f9-dirty` |
| branch | `dagbft-integration` |
| uncommitted files | 269 (see config/uncommitted.patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:d6056e232b69b1f8f66a07cb364b650c807b1fca616ebf81abe34c5548becbc4` |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| synthetic drops | `` |
| anchor drops | `none` |
| topology | 2 BVNs, 8 nodes + bootstrap |
| partitions | Directory BVN1 BVN2 |
| chaos | off |
| target duration | 12h |
| target TPS | 500 |
| storage |  (docker-network.yml) |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/` (soak.conf, the compose and network files). Results appended below on exit.

## Stopped early (2026-09-15, ~2.2h of 12h)

Stopped to work #4280. **Chaos was OFF for this run** — the launch passed
`CHAOS=on` in the environment, which soak.conf overrides by design, and the
manifest above records the truth. So everything below is a fault-free network
under load: no restarts, no pauses, nothing dropped by the harness.

What it found, which a chaos run would have masked: healing pulled 743,000
entries against 23,000 requests with no faults on the network — about 56% of
all traffic those streams ever carried, to cover ~1% in flight (#4280). Also
measured: execution lag 75-184 blocks on both BVNs against a bound of 8, which
throttled accepted load from 500 to ~307 tps; 300 queue-full dispatcher drops;
542 heal misses caused by the destination asking from a Delivered far enough
behind that the producer cache had released the span.

Evidence kept: `metrics-*-at-stop.txt`, `soakmon-at-stop.json`.
