# Soak run 20261003T232435Z

**Purpose:** R8-1: restart test with twelve single-validator restarts on lead bb77f9222 (#4515 release fix)

| field | value |
|---|---|
| started (UTC) | 2026-10-03T23:24:35Z |
| commit | `bb77f92225a363181f368740f5fe4d5a14cbc74f` |
| describe | `backup/dta-e11-before-lead-sync-573-gbb77f9222` |
| branch | `issue-4205-lead` |
| uncommitted files | 5425: 0 tracked, 5425 untracked (in no patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:33e34e23cfacfa8bfb42afa0e71686b91a0705d1e90175bbacc48e275594e4da` |
| late follower image id (acc-bvn3-fol2) | `disoak-bvn3-fol2` `sha256:45e72ecf759608cb0d3667178c5ee999b29ffc2bd75e41d8031176e7e79e73cf` (after the build) |
| late follower image id (acc-bvn2-fol1) | `disoak-bvn2-fol1` `sha256:9a28b4ff43d2ee8c0f730a4dcc7abdf6daa2fb908261d4ff1122cebbcc974b84` (after the build) |
| late follower image id (acc-bvn1-fol1) | `disoak-bvn1-fol1` `sha256:89c71643c184e3ff7bac6f5d9ff0c716139904725b7eb123ef21afd4214766ca` (after the build) |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| fault model | none (CHAOS=off) |
| topology | 3 BVNs, 12 validators + 1 follower (acc-bvn3-fol1, partitions Directory BVN3) + bootstrap |
| follower key | inactive in the definition; active validators per partition: BVN1 4, BVN2 4, BVN3 4, Directory 12 |
| partitions | Directory BVN1 BVN2 BVN3 |
| chaos | off |
| target duration | 72h |
| target TPS | 1 |
| storage | BlockchainDB (docker-network.yml) |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1300MiB (effective, from docker inspect) |

Config as run is frozen in `config/` (soak.conf + override.conf, the compose and network files). Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-10-04T00:25:26Z |
| elapsed | 0.97h |
| driver exit | 1 (FAILED) |
| Directory height (block, the highest any of its validators that answered executed; first -> last sample) | 40 -> 3414 |
| heals | 0 -> 0 |
| chaos events | 63 |
| monitor samples | 109 |
| seizure | SEIZED at 2026-10-03T23:41:12 :: stalled stream, undelivered for 20 polls :: stuck=n/a stuckStream= worst=BVN1->Directory gap=0 deliv=48 undeliv=synthetic BVN2->BVN3 undeliv=3 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 7 |
| load generator reads a node would not answer (whole run) | NotReady: 2247 answers retried at another endpoint, 73 queries no endpoint would answer; transport error: 3704 answers retried at another endpoint, 182 queries failed at every endpoint |
| read-back probe (the validators, whole run) | Whole run: 15238 timed reads, p50 1.0 ms, p95 2.0 ms, p99 12.6 ms, max 63.2 ms (txn read, BVN1, entry 2634 blocks old); 350 failed (0 of them timed out, 8s), 514 refused NotReady (a joining node's designed answer; not timed), 0 refused by the API's query gate (not timed). |
| follower read probe | acc-bvn1-fol1: declared (late-follower profile) and not running at any probe round — not a follower of this run; not probed. |
| wedge captures (a partition closed no block for 120s; #4125) | 1 wedge-20261003T235240Z |
| delivery-stall captures (every partition kept closing blocks while a synthetic flow into one was red for 120s; #4285) | 2 delivery-stall-20261003T235704Z,delivery-stall-20261004T001239Z |
| accepted, neither certified here, taken on relay, nor refused (#, whole run, the validators) | 24249, worst 2588 on acc-bvn2-val3/BVN2 (as of 2026-10-04T00:25:24Z; rising 22745 -> 24249 over the last 5 samples; final row written, 0s after the load generator exited; 14 samples skipped as incomplete (a node answered no scrape); 78 counter resets carried forward (4284 stranded before a restart; the final row's own readings sum to 19965)) |
| restarted node rejoined (per node and partition: gauge ACTIVE, executed block within 5 of the highest block any of the partition's answering validators executed, through its last reading, and every anchor it stated agreeing with its peers'; s = container start to the first sample ACTIVE and within that bound; the validators) | rejoined 76 of 78 start(s) after the launch, worst 93.0s from container start to executing with its partition (acc-bvn2-val3 directory); NOT rejoined: acc-bvn2-val4 directory (gauge ACTIVE at 34.7s; anchor disagrees with its peers: block 2650 root 72aec74f, its peers' 72aec74f (and 4 more)); acc-bvn3-val2 bvn3 (gauge ACTIVE at 30.6s; anchor disagrees with its peers: block 2884 root af748e3e, its peers' 25e27913 (and 5 more)); 24 started before the monitor's first sample (the network's launch: not judged) |

### Follower (#4365)

| what | value |
|---|---|
| follower | `acc-bvn3-fol1`, partitions BVN3, Directory |
| follower key in the NetworkDefinition | inactive in the definition |
| active validators per partition, from the NetworkDefinition | BVN1 4, BVN2 4, BVN3 4, Directory 12 |
| committee size per partition (validators, at genesis) | BVN1 4, BVN2 4, BVN3 4, Directory 12 |
| committees that disagreed across nodes (#) | 0 |
| follower in no committee | yes |
| validators added to a committee during the run (#) | 0 |
| anchors the follower dispatched (#4367: must be 0) (#) | 0 |
| blocks the follower stated a root for without sending (#) | 4041 |
| anchored blocks compared, follower vs a validator (#) | 4033 |
| root/BPT mismatches (#) | 0 |
| first mismatching block | none |
| blocks the follower anchored that no validator had (#) | 0 |
| anchor lines carrying no source partition (#4370) (#) | 0 |
| blocks where the follower contradicted itself (#) | 0 |
| certificates refused for a non-committee author (#) | 0 |
| ...of those, authored by the follower (#) | 0 |
| headers dropped by validators for a non-committee author (#) | — not measured (`Header from unknown validator` and `Vote from unknown validator` are slog.Debug with no `module` attribute (vote_handler.go:34,284); the generated logging config sets Debug per module and Info as the default, so this build emits neither line) |
| votes dropped by validators for a non-committee author (#) | — not measured |
| follower behind the validators (blocks, max over the run) | 0, at 2026-10-03T23:26:54Z on BVN3 — the monitor's high-water mark over every tick |
| follower behind the validators (blocks, at the last sample) | BVN3 0, Directory 0 |
| samples where the follower did not answer (#) | 4 |
| accepted, neither certified here, taken on relay, nor refused (#, whole run) | 80, worst 80 on acc-bvn3-fol1/BVN3 (as of 2026-10-04T00:25:24Z; rising 63 -> 80 over the last 5 samples; final row written, 0s after the load generator exited; 2 counter resets carried forward (1 stranded before a restart; the final row's own readings sum to 79); 1 (node, partition) never submitted to this run, counted 0 (acc-bvn3-fol1/Directory)) |
| relayed (#, whole run) | 4669 taken / 0 (series present) refused / 79 target not ready / 0 (series present) unreachable (as of 2026-10-04T00:25:24Z) |
| follower start rejoined (per partition, the add-follower and every restart; same reading as the validators' row) | rejoined 4 of 4 start(s) after the launch, worst 16.4s from container start to executing with its partition (acc-bvn3-fol1 bvn3); 2 started before the monitor's first sample (the network's launch: not judged) |

**Stranded across disturbances (#4364).** The criterion is that the
figure does not climb between disturbances and that every step is
attributable to one of them — these counters never clear, so this is
a cumulative loss, not a level. Two different numbers, so both are
here: a **step** is what a disturbance cost, and a **creep** is the
climb in the quiet stretch between two of them. The criterion's
number is `largest climb between disturbances`.

Settled means the MINIMUM over a window, because the figure jitters
by whatever is in flight between samples and a single reading is not
a level. The windows are LOCAL — 120s either
side of the disturbance, four samples at the 30s cadence and well
inside the chaos cadence — so a rise in the middle of a quiet
stretch is a creep and not the next disturbance's step. The
after-window starts 60s late, because the
sample at the disturbance's own second still reads the pre-effect
level: a relay has to time out before it gives up, so the loss
shows a sample later and a floor taken from that second would
print the step as 0 and the loss as the creep after it. The sample
AT the disturbance is in neither window. A window clamped by a
neighbouring event says so, and there the step and the creep beside
it are not separable. A pause is dated at its un-pause.

| disturbance | stranded |
|---|---|
| baseline (the first 120s of the run) | 0 |
| between the run's start and 23:33Z | crept +0 |
| 23:33Z restart complete | stranded 0 -> 1 (+1) |
| between 23:33Z and 23:39Z | crept +0 |
| 23:39Z restart complete | stranded 1 -> 1 (+0) |
| between 23:39Z and the end of the run | crept +62 |
| largest step at a disturbance | +1, at 23:33Z restart complete |
| largest climb between disturbances | +62, 23:39Z to the end of the run |
| counter resets seen | 2 (acc-bvn3-fol1/BVN3 at 23:33Z carrying 0, acc-bvn3-fol1/BVN3 at 23:39Z carrying 1) — the carry is the floor of the pair's last samples, not its last reading, and the series does not step down |

**Per add-follower and remove-follower (#4364).** Each add is read
over that container's life only, from its `add-follower` line to
its `remove-follower`; a quantity the life did not record reads
`not measured`. A removal is judged on the readings
30s either side of it.

| event | verdict |
|---|---|
| add-follower / remove-follower | none in `chaos.log` |

Full detail in `follower-report.md`; the per-sample series in `follower.csv`.

Raw: `soak.log`, `monitor.csv`, `mem.csv` (every node, with its role), `submissions.csv`, `chaos.log`, `nodestate.csv`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`, `follower.csv` / `follower-report.md`, `readprobe-follower.csv`, `follower-removal-N-{before,at,after}.json`, `network-definition.json`.
