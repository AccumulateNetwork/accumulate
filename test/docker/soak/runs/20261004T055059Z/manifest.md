# Soak run 20261004T055059Z

**Purpose:** R9-4: restart test with twelve single restarts on lead a38a66290 (#4517 anchor delivery fixes)

| field | value |
|---|---|
| started (UTC) | 2026-10-04T05:50:59Z |
| commit | `a38a66290057c1e9d22f25638ce929d82e134c10` |
| describe | `backup/dta-e11-before-lead-sync-583-ga38a66290` |
| branch | `issue-4205-lead` |
| uncommitted files | 5558: 0 tracked, 5558 untracked (in no patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:589ee7662bb400e052a8beb9781ecc09f4401bbd7fc1f17307b56cfea3b16993` |
| late follower image id (acc-bvn3-fol2) | `disoak-bvn3-fol2` `sha256:0a525a853bd06e696c2c05c6a87f36c459fcf8c0862d06dceb3b05e780e62b41` (after the build) |
| late follower image id (acc-bvn2-fol1) | `disoak-bvn2-fol1` `sha256:5351e333655241e85bd15375a0fdcc735d728989fca7b4ff3cc2e846d7311980` (after the build) |
| late follower image id (acc-bvn1-fol1) | `disoak-bvn1-fol1` `sha256:f8f2ccca55c365b4da23bd857cccc0bf489834d4119b8569dcbe59c35b805aff` (after the build) |
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
| ended (UTC) | 2026-10-04T06:46:25Z |
| elapsed | 0.87h |
| driver exit | 1 (FAILED) |
| Directory height (block, the highest any of its validators that answered executed; first -> last sample) | 40 -> 3056 |
| heals | 0 -> 0 |
| chaos events | 63 |
| monitor samples | 98 |
| seizure | SEIZED at 2026-10-04T06:08:09 :: stalled stream, undelivered for 20 polls :: stuck=n/a stuckStream= worst=BVN1->Directory gap=0 deliv=92 undeliv=synthetic BVN2->Directory undeliv=2 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 0 |
| load generator reads a node would not answer (whole run) | NotReady: 851 answers retried at another endpoint, 47 queries no endpoint would answer; transport error: 4720 answers retried at another endpoint, 282 queries failed at every endpoint |
| read-back probe (the validators, whole run) | Whole run: 13875 timed reads, p50 0.8 ms, p95 2.0 ms, p99 10.6 ms, max 49.3 ms (txn read, BVN2, entry 1420 blocks old); 613 failed (0 of them timed out, 8s), 93 refused NotReady (a joining node's designed answer; not timed), 0 refused by the API's query gate (not timed). |
| follower read probe | acc-bvn1-fol1: declared (late-follower profile) and not running at any probe round — not a follower of this run; not probed. |
| wedge captures (a partition closed no block for 120s; #4125) | 1 wedge-20261004T061950Z |
| delivery-stall captures (every partition kept closing blocks while a synthetic flow into one was red for 120s; #4285) | 1 delivery-stall-20261004T061907Z |
| accepted, neither certified here, taken on relay, nor refused (#, whole run, the validators) | 102, worst 14 on acc-bvn3-val4/BVN3 (as of 2026-10-04T06:46:24Z; falling 154 -> 102 over the last 5 samples (draining); final row written, 0s after the load generator exited; 15 samples skipped as incomplete (a node answered no scrape); 78 counter resets carried forward (98 stranded before a restart; the final row's own readings sum to 4)) |
| restarted node rejoined (per node and partition: gauge ACTIVE, executed block within 5 of the highest block any of the partition's answering validators executed, through its last reading, and every anchor it stated agreeing with its peers'; s = container start to the first sample ACTIVE and within that bound; the validators) | rejoined 76 of 78 start(s) after the launch, worst 97.5s from container start to executing with its partition (acc-bvn2-val3 directory); NOT rejoined: acc-bvn2-val1 bvn2 (gauge ACTIVE at 20.8s; anchor disagrees with its peers: block 2017 root 6af1c171, its peers' d081118a (and 5 more)); acc-bvn3-val1 bvn3 (gauge ACTIVE at 26.0s; anchor disagrees with its peers: block 2604 root d25bea59, its peers' 6ff178f3 (and 1 more)); 24 started before the monitor's first sample (the network's launch: not judged) |

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
| blocks the follower stated a root for without sending (#) | 3822 |
| anchored blocks compared, follower vs a validator (#) | 3817 |
| root/BPT mismatches (#) | 1 |
| first mismatching block | BVN3 block 2604: follower root 6ff178f3 / bpt aab9db80, `acc-bvn3-val1` root d25bea59 / bpt b791f04f |
| blocks the follower anchored that no validator had (#) | 0 |
| anchor lines carrying no source partition (#4370) (#) | 0 |
| blocks where the follower contradicted itself (#) | 0 |
| certificates refused for a non-committee author (#) | 0 |
| ...of those, authored by the follower (#) | 0 |
| headers dropped by validators for a non-committee author (#) | — not measured (`Header from unknown validator` and `Vote from unknown validator` are slog.Debug with no `module` attribute (vote_handler.go:34,284); the generated logging config sets Debug per module and Info as the default, so this build emits neither line) |
| votes dropped by validators for a non-committee author (#) | — not measured |
| follower behind the validators (blocks, max over the run) | 0, at 2026-10-04T05:53:51Z on BVN3 — the monitor's high-water mark over every tick |
| follower behind the validators (blocks, at the last sample) | BVN3 0, Directory 0 |
| samples where the follower did not answer (#) | 4 |
| accepted, neither certified here, taken on relay, nor refused (#, whole run) | 42, worst 42 on acc-bvn3-fol1/BVN3 (as of 2026-10-04T06:46:24Z; rising 26 -> 42 over the last 5 samples; final row written, 0s after the load generator exited; 2 counter resets carried forward (1 stranded before a restart; the final row's own readings sum to 41); 1 (node, partition) never submitted to this run, counted 0 (acc-bvn3-fol1/Directory)) |
| relayed (#, whole run) | 4591 taken / 0 (series present) refused / 41 target not ready / 0 (series present) unreachable (as of 2026-10-04T06:46:24Z) |
| follower start rejoined (per partition, the add-follower and every restart; same reading as the validators' row) | rejoined 4 of 4 start(s) after the launch, worst 14.5s from container start to executing with its partition (acc-bvn3-fol1 directory); 2 started before the monitor's first sample (the network's launch: not judged) |

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
| between the run's start and 06:00Z | crept +0 |
| 06:00Z restart complete | stranded 0 -> 1 (+1) |
| between 06:00Z and 06:05Z | crept +0 |
| 06:05Z restart complete | stranded 1 -> 1 (+0) |
| between 06:05Z and the end of the run | crept +30 |
| largest step at a disturbance | +1, at 06:00Z restart complete |
| largest climb between disturbances | +30, 06:05Z to the end of the run |
| counter resets seen | 2 (acc-bvn3-fol1/BVN3 at 06:00Z carrying 0, acc-bvn3-fol1/BVN3 at 06:06Z carrying 1) — the carry is the floor of the pair's last samples, not its last reading, and the series does not step down |

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
