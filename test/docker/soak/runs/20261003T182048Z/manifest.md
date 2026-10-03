# Soak run 20261003T182048Z

**Purpose:** restart and quorum-loss test on lead 84f951d33

| field | value |
|---|---|
| started (UTC) | 2026-10-03T18:20:48Z |
| commit | `84f951d33248b35a6b7f5fa6cd4667fd8aa3d337` |
| describe | `backup/dta-e11-before-lead-sync-565-g84f951d33` |
| branch | `issue-4205-lead` |
| uncommitted files | 5213: 0 tracked, 5213 untracked (in no patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:1d403c556866d814c7b2ac8cd259cc636380846f77bee0b74368452244ac2fd9` |
| late follower image id (acc-bvn3-fol2) | `disoak-bvn3-fol2` `sha256:711bbbf095eb7620c874f7c72bcf52ded495581b0b496ba7d7593af3cc533854` (after the build) |
| late follower image id (acc-bvn2-fol1) | `disoak-bvn2-fol1` `sha256:0050b0c80d0099db4bf82e3aa904c6fb08096dc7145e44bab7ba9423573a7a45` (after the build) |
| late follower image id (acc-bvn1-fol1) | `disoak-bvn1-fol1` `sha256:b93ba0c15a79bd6a185260611a0f02f9d07fe5de05886f7cefd664906556955d` (after the build) |
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
| ended (UTC) | 2026-10-03T19:16:26Z |
| elapsed | 0.87h |
| driver exit | 1 (FAILED) |
| Directory height (block, the highest any of its validators that answered executed; first -> last sample) | 40 -> 3083 |
| heals | 0 -> 290 |
| chaos events | 35 |
| monitor samples | 98 |
| seizure | SEIZED at 2026-10-03T18:45:22 :: stalled stream, undelivered for 20 polls :: stuck=n/a stuckStream= worst=BVN2->Directory gap=3 deliv=341 undeliv=synthetic BVN3->BVN1 undeliv=3 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 3 |
| load generator reads a node would not answer (whole run) | NotReady: 1290 answers retried at another endpoint, 108 queries no endpoint would answer; transport error: 3204 answers retried at another endpoint, 165 queries failed at every endpoint |
| read-back probe (the validators, whole run) | Whole run: 12925 timed reads, p50 0.7 ms, p95 2.1 ms, p99 10.8 ms, max 43.7 ms (txn read, Directory, entry 3078 blocks old); 300 failed (0 of them timed out, 8s), 995 refused NotReady (a joining node's designed answer; not timed), 0 refused by the API's query gate (not timed). |
| follower read probe | acc-bvn1-fol1: declared (late-follower profile) and not running at any probe round — not a follower of this run; not probed. |
| wedge captures (a partition closed no block for 120s; #4125) | 1 wedge-20261003T185731Z |
| delivery-stall captures (every partition kept closing blocks while a synthetic flow into one was red for 120s; #4285) | 1 delivery-stall-20261003T190135Z |
| accepted, neither certified here, taken on relay, nor refused (#, whole run, the validators) | 4073, worst 2708 on acc-bvn2-val4/Directory (as of 2026-10-03T19:16:23Z; rising 3784 -> 4073 over the last 5 samples; final row written, 0s after the load generator exited; 12 samples skipped as incomplete (a node answered no scrape); 54 counter resets carried forward (1477 stranded before a restart; the final row's own readings sum to 2596)) |
| restarted node rejoined (per node and partition: gauge ACTIVE, executed block within 5 of the highest block any of the partition's answering validators executed, through its last reading, and every anchor it stated agreeing with its peers'; s = container start to the first sample ACTIVE and within that bound; the validators) | rejoined 53 of 54 start(s) after the launch, worst 58.1s from container start to executing with its partition (acc-bvn2-val4 directory); NOT rejoined: acc-bvn1-val3 directory (never ACTIVE (spine stalled at entry 654); executed 723 vs partition 3103 at its last reading (2380 behind; bound 5; 11 other validators answered)); 24 started before the monitor's first sample (the network's launch: not judged) |

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
| blocks the follower stated a root for without sending (#) | 3955 |
| anchored blocks compared, follower vs a validator (#) | 3944 |
| root/BPT mismatches (#) | 0 |
| first mismatching block | none |
| blocks the follower anchored that no validator had (#) | 0 |
| anchor lines carrying no source partition (#4370) (#) | 0 |
| blocks where the follower contradicted itself (#) | 0 |
| certificates refused for a non-committee author (#) | 0 |
| ...of those, authored by the follower (#) | 0 |
| headers dropped by validators for a non-committee author (#) | — not measured (`Header from unknown validator` and `Vote from unknown validator` are slog.Debug with no `module` attribute (vote_handler.go:34,284); the generated logging config sets Debug per module and Info as the default, so this build emits neither line) |
| votes dropped by validators for a non-committee author (#) | — not measured |
| follower behind the validators (blocks, max over the run) | 0, at 2026-10-03T18:23:33Z on BVN3 — the monitor's high-water mark over every tick |
| follower behind the validators (blocks, at the last sample) | BVN3 0, Directory 0 |
| samples where the follower did not answer (#) | 2 |
| accepted, neither certified here, taken on relay, nor refused (#, whole run) | 1, worst 1 on acc-bvn3-fol1/BVN3 (as of 2026-10-03T19:16:23Z; flat at 1 -> 1 over the last 5 samples; final row written, 0s after the load generator exited; 1 sample skipped as incomplete (a node answered no scrape); 2 counter resets carried forward (1 stranded before a restart; the final row's own readings sum to 0); 1 (node, partition) never submitted to this run, counted 0 (acc-bvn3-fol1/Directory)) |
| relayed (#, whole run) | 5858 taken / 0 (series present) refused / 0 (series present) target not ready / 0 (series present) unreachable (as of 2026-10-03T19:16:23Z) |
| follower start rejoined (per partition, the add-follower and every restart; same reading as the validators' row) | rejoined 4 of 4 start(s) after the launch, worst 16.9s from container start to executing with its partition (acc-bvn3-fol1 bvn3); 2 started before the monitor's first sample (the network's launch: not judged) |

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
| between the run's start and 18:30Z | crept +0 |
| 18:30Z restart complete | stranded 0 -> 1 (+1) |
| between 18:30Z and 18:35Z | crept +0 |
| 18:35Z restart complete | stranded 1 -> 1 (+0) |
| between 18:35Z and the end of the run | crept +0 |
| largest step at a disturbance | +1, at 18:30Z restart complete |
| largest climb between disturbances | +0, the run's start to 18:30Z — the figure did not climb |
| samples dropped as incomplete | 1 of 95 — a node answered no scrape at those (every one of its rows empty; a node empty on one partition only is joining it and counts 0), and a fleet total missing one node dips by that node's count |
| counter resets seen | 2 (acc-bvn3-fol1/BVN3 at 18:30Z carrying 0, acc-bvn3-fol1/BVN3 at 18:35Z carrying 1) — the carry is the floor of the pair's last samples, not its last reading, and the series does not step down |

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
