# Soak run 20260924T093936Z

**Purpose:** 30m 100tps chaos on issue-4205-lead be4ddd0ca: third Docker run of the second-pass join. On top of the second run (20260924T074702Z): #4385 (ACTIVE only from a successful handoff; demoted to BOOTING on a re-sync or a failed handoff; every service and the gauge follow one state), #4415 (healing pair drawn from the root anchor as written; stillness counted on every node), #4413 (no unsigned anchors served), #4414 harness. NOT yet: #4411/#4412 (further-behind joins, decisions), #4416/#4418/#4421 (in review/build). Expected: barely-behind BVN restarts rejoin; further-behind and Directory sides still fail but no stale-ACTIVE node serves or strands. Launched by the interactive session, Paul watching via 100.75.75.92:8098.

| field | value |
|---|---|
| started (UTC) | 2026-09-24T09:39:36Z |
| commit | `be4ddd0ca8fdc9ab9a3e92c23c4252dec7e7a5d7` |
| describe | `backup/dta-e11-before-lead-sync-219-gbe4ddd0ca` |
| branch | `issue-4205-lead` |
| uncommitted files | 165: 0 tracked, 165 untracked (in no patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:bdfd64ae9dd94fd9ea47fcc793c9ce91674d64b7e41fb86b8340ddcb378674af` |
| late follower image id (acc-bvn3-fol2) | `disoak-bvn3-fol2` `sha256:4abdbc756403adf8e2e15dd9fde1b164191e1f2f71d24009b85f45f6b235f934` (after the build) |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| fault model | restart or pause one BVN validator container every 120s + 0-60s |
| topology | 3 BVNs, 12 validators + 1 follower (acc-bvn3-fol1, partitions Directory BVN3) + bootstrap |
| follower key | inactive in the definition; active validators per partition: BVN1 4, BVN2 4, BVN3 4, Directory 12 |
| partitions | Directory BVN1 BVN2 BVN3 |
| chaos | on |
| target duration | 30m |
| target TPS | 100 |
| storage | BlockchainDB (docker-network.yml) |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/` (soak.conf + override.conf, the compose and network files). Results appended below on exit.

## Stopped early by stallkill

- stopped (UTC): 2026-09-24T10:08:24Z
- reason: stalled 248s: BVN3 (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-24T10:08:27Z |
| elapsed | 0.45h |
| driver exit | 143 (FAILED) |
| Directory height (block, the highest any of its validators that answered executed; first -> last sample) | 40 -> 1590 |
| heals | 0 -> 8405 |
| chaos events | 19 |
| monitor samples | 51 |
| seizure | SEIZED at 2026-09-24T09:44:09 :: stuck=n/a stuckStream= worst=BVN2->BVN1 gap=136 deliv=1920 undeliv=synthetic BVN2->BVN1 undeliv=102 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 9 |
| load generator reads a node would not answer (whole run) | NotReady: 43823 answers retried at another endpoint, 0 queries no endpoint would answer; transport error: 476 answers retried at another endpoint, 0 queries failed at every endpoint |
| read-back probe | Whole run: 5744 timed reads, p50 1.6 ms, p95 7.7 ms, p99 32.7 ms, max 8015.2 ms (chain read, BVN3, entry 688 blocks old); 1541 failed, 25 timed out (8s), 0 refused by the API's query gate (not timed). |
| follower read probe | acc-bvn3-fol1 (partitions Directory, BVN3): 1429 reads of entries it holds, 1429 answered, 0 refused (query gate or NotReady), 0 failed; p50 0.5 ms, p95 1.8 ms, max 177.9 ms. |
| wedge captures (a partition closed no block for 120s; #4125) | 0  |
| delivery-stall captures (every partition kept closing blocks while a synthetic flow into one was red for 120s; #4285) | 1 delivery-stall-20260924T100245Z |
| accepted, neither certified here, taken on relay, nor refused (#, whole run, the validators) | 181, worst 28 on acc-bvn2-val1/BVN2 (as of 2026-09-24T10:06:11Z; rising 146 -> 181 over the last 5 samples; FINAL ROW MISSING — soakmon's exit write did not land; this reading is mid-drain and up to 30s stale, but 133s BEFORE the load generator exited — mid-drain; 8 samples skipped as incomplete (a node answered no scrape) — INCLUDING THE LAST, so this is not the final row; 8 counter resets carried forward (39 stranded before a restart; the final row's own readings sum to 142); 3 (node, partition) joining at this sample, counted 0 (acc-bvn1-val3/Directory, acc-bvn2-val2/Directory, acc-bvn3-val1/Directory); the run was stopped by stallkill, so the load generator was killed mid-flight and this is NOT a drained sample) |
| restarted node rejoined (per node and partition: gauge ACTIVE, executed block within 5 of the highest block any of the partition's answering validators executed, through its last reading, and every anchor it stated agreeing with its peers'; s = container start to the first sample ACTIVE and within that bound; the validators) | rejoined 1 of 8 start(s) after the launch, worst 15.7s from container start to executing with its partition (acc-bvn1-val1 bvn1); NOT rejoined: acc-bvn1-val1 directory (never ACTIVE (BOOTING); executed 206 vs partition 1579 at its last reading (1373 behind; bound 5; 10 other validators answered); anchor agreement not measured: it stated no directory anchor after its start); acc-bvn1-val3 bvn1 (never ACTIVE (BOOTING); executed 1328 vs partition 1592 at its last reading (264 behind; bound 5; 3 other validators answered); anchor agreement not measured: it stated no bvn1 anchor after its start); acc-bvn1-val3 directory (never ACTIVE (BOOTING); executed 1328 vs partition 1579 at its last reading (251 behind; bound 5; 10 other validators answered); anchor agreement not measured: it stated no directory anchor after its start); acc-bvn2-val2 bvn2 (never ACTIVE (BOOTING); executed 1001 vs partition 1577 at its last reading (576 behind; bound 5; 2 other validators answered); anchor agreement not measured: it stated no bvn2 anchor after its start); acc-bvn2-val2 directory (never ACTIVE (BOOTING); executed 998 vs partition 1579 at its last reading (581 behind; bound 5; 10 other validators answered); anchor agreement not measured: it stated no directory anchor after its start); acc-bvn3-val1 bvn3 (never ACTIVE (BOOTING); executed 582 vs partition 1607 at its last reading (1025 behind; bound 5; 3 other validators answered); anchor agreement not measured: it stated no bvn3 anchor after its start); acc-bvn3-val1 directory (never ACTIVE (BOOTING); executed 576 vs partition 1579 at its last reading (1003 behind; bound 5; 10 other validators answered); anchor agreement not measured: it stated no directory anchor after its start); 24 started before the monitor's first sample (the network's launch: not judged) |

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
| blocks the follower stated a root for without sending (#) | 2845 |
| anchored blocks compared, follower vs a validator (#) | 2845 |
| root/BPT mismatches (#) | 0 |
| first mismatching block | none |
| blocks the follower anchored that no validator had (#) | 0 |
| anchor lines carrying no source partition (#4370) (#) | 0 |
| blocks where the follower contradicted itself (#) | 0 |
| certificates refused for a non-committee author (#) | 0 |
| ...of those, authored by the follower (#) | 0 |
| headers dropped by validators for a non-committee author (#) | — not measured (`Header from unknown validator` and `Vote from unknown validator` are slog.Debug with no `module` attribute (vote_handler.go:34,284); the generated logging config sets Debug per module and Info as the default, so this build emits neither line) |
| votes dropped by validators for a non-committee author (#) | — not measured |
| follower behind the validators (blocks, max over the run) | 1, at 2026-09-24T09:51:50Z on BVN3 — the monitor's high-water mark over every tick |
| follower behind the validators (blocks, at the last sample) | BVN3 0, Directory 0 |
| samples where the follower did not answer (#) | 0 |
| accepted, neither certified here, taken on relay, nor refused (#, whole run) | 0, worst 0 on acc-bvn3-fol1/BVN3 (as of 2026-09-24T10:08:24Z; 0 at the last sample; final row written, 0s after the load generator exited; 1 (node, partition) never submitted to this run, counted 0 (acc-bvn3-fol1/Directory); the run was stopped by stallkill, so the load generator was killed mid-flight and this is NOT a drained sample) |
| relayed (#, whole run) | 9773 taken / 0 (series present) refused / 0 (series present) target not ready / 0 (series present) unreachable (as of 2026-09-24T10:08:24Z) |
| follower start rejoined (per partition, the add-follower and every restart; same reading as the validators' row) | no start after the network's launch; 2 started before the monitor's first sample (the network's launch: not judged) |

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
| between the run's start and 09:43Z | crept +0 |
| 09:43Z restart acc-bvn1-val1 | stranded 0 -> 0 (+0) |
| between 09:43Z and 09:46Z | crept +0 |
| 09:46Z pause acc-bvn2-val1 | stranded 0 -> 0 (+0) |
| between 09:46Z and 09:50Z | crept +0 |
| 09:50Z restart acc-bvn3-val1 | stranded 0 -> 0 (+0) |
| between 09:50Z and 09:53Z | crept +0 |
| 09:53Z pause acc-bvn1-val2 | stranded 0 -> 0 (+0) |
| between 09:53Z and 09:57Z | crept +0 |
| 09:57Z restart acc-bvn2-val2 | stranded 0 -> 0 (+0) |
| between 09:57Z and 10:00Z | crept +0 |
| 10:00Z pause acc-bvn3-val2 | stranded 0 -> 0 (+0) |
| between 10:00Z and 10:03Z | crept +0 |
| 10:03Z restart acc-bvn1-val3 | stranded 0 -> 0 (+0) |
| between 10:03Z and 10:06Z | crept +0 |
| 10:06Z pause acc-bvn2-val3 | — not measured (no complete sample in the 120s before, or between the settle and the next disturbance) (window shortened by the run's start or end) |
| largest step at a disturbance | +0, at 09:43Z restart acc-bvn1-val1 — no disturbance cost anything (1 of 8 disturbances not measured) |
| largest climb between disturbances | +0, the run's start to 09:43Z — the figure did not climb (1 of 8 disturbances not measured) |

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
