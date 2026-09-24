# Soak run 20260924T111811Z

**Purpose:** 30m 100tps chaos on issue-4205-lead 3a51d6d3f: fourth Docker run of the second-pass join. On top of the third (20260924T093936Z): #4423 (a second copy of a held synthetic entry records nothing — the third run stop cause), #4421 (spine pulled whole every pass; messageless stores repaired; a diverged chain retaken whole, provisional), #4416 (pull rebuilds an anchor history, signer, sequenced message, cause and validator-signature set), #4418/#4419 (bodiless entries refused, stalled spine visible), #4424 (no outsider signature in anchor heals; synthetics served by any holder), #4426 (a validation refusal is answered as the refusal), #4425 harness. NOT yet: #4411/#4412 (further-behind joins, decisions). Expected: no delivery freeze; barely-behind and Directory restarts with handoffs no longer failing on missing messages; further-behind joins still fail on #4411. Launched by the interactive session, Paul watching via 100.75.75.92:8098.

| field | value |
|---|---|
| started (UTC) | 2026-09-24T11:18:12Z |
| commit | `3a51d6d3f2d4c4433b7e0ec1054b320ee13338b6` |
| describe | `backup/dta-e11-before-lead-sync-283-g3a51d6d3f` |
| branch | `issue-4205-lead` |
| uncommitted files | 262: 0 tracked, 262 untracked (in no patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:4d81e804daf461bd41ba40072d90c9406251d55d87ac17fc2ff6df163398b256` |
| late follower image id (acc-bvn3-fol2) | `disoak-bvn3-fol2` `sha256:c4d60287daf1e5dacf752cc7e0e57621b7bca0548aa7222c1a7a133dda104aec` (after the build) |
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

- stopped (UTC, the decision): 2026-09-24T11:43:03Z
- evidence capture ended, load generator signalled (UTC): 2026-09-24T11:43:36Z (33 s after the decision)
- reason: stalled 246s: BVN1,BVN2,BVN3,Directory (threshold 240s)

Evidence was captured from the decision on; see the probe-* directory
written as the capture began. The network ran on, under load and chaos,
until the capture ended.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-24T11:43:38Z |
| elapsed | 0.39h |
| driver exit | 143 (FAILED) |
| Directory height (block, the highest any of its validators that answered executed; first -> last sample) | 40 -> 1365 |
| heals | 0 -> 9168 |
| chaos events | 15 |
| monitor samples | 45 |
| seizure | SEIZED at 2026-09-24T11:21:55 :: stuck=n/a stuckStream= worst=BVN2->BVN1 gap=114 deliv=1497 undeliv=synthetic BVN2->BVN3 undeliv=104 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 9 |
| load generator reads a node would not answer (whole run) | NotReady: 36691 answers retried at another endpoint, 0 queries no endpoint would answer; transport error: 382 answers retried at another endpoint, 0 queries failed at every endpoint |
| read-back probe (the validators, whole run) | Whole run: 3275 timed reads, p50 1.0 ms, p95 4.1 ms, p99 8007.4 ms, max 8008.6 ms (txn read, BVN2, entry 329 blocks old); 36 failed (36 of them timed out, 8s), 1179 refused NotReady (a joining node's designed answer; not timed), 0 refused by the API's query gate (not timed). |
| follower read probe | acc-bvn3-fol1 (partitions Directory, BVN3): 1116 reads of entries it holds, 1116 answered, 0 refused (query gate or NotReady), 0 failed; p50 0.4 ms, p95 1.1 ms, max 10.2 ms. |
| wedge captures (a partition closed no block for 120s; #4125) | 0  |
| delivery-stall captures (every partition kept closing blocks while a synthetic flow into one was red for 120s; #4285) | 1 delivery-stall-20260924T114143Z |
| accepted, neither certified here, taken on relay, nor refused (#, whole run, the validators) | 116, worst 18 on acc-bvn1-val2/BVN1 (as of 2026-09-24T11:43:36Z; rising 49 -> 116 over the last 5 samples; final row written, 0s after the load generator exited; 1 sample skipped as incomplete (a node answered no scrape); 6 counter resets carried forward (14 stranded before a restart; the final row's own readings sum to 102); 2 (node, partition) joining at this sample, counted 0 (acc-bvn2-val2/Directory, acc-bvn3-val1/Directory); the run was stopped by stallkill, so the load generator was killed mid-flight and this is NOT a drained sample) |
| restarted node rejoined (per node and partition: gauge ACTIVE, executed block within 5 of the highest block any of the partition's answering validators executed, through its last reading, and every anchor it stated agreeing with its peers'; s = container start to the first sample ACTIVE and within that bound; the validators) | rejoined 0 of 6 start(s) after the launch; NOT rejoined: acc-bvn1-val1 bvn1 (never ACTIVE (BOOTING); executed 157 vs partition 1420 at its last reading (1263 behind; bound 5; 3 other validators answered); anchor agreement not measured: it stated no bvn1 anchor after its start); acc-bvn1-val1 directory (gauge ACTIVE at 27.3s; executed 482 vs partition 1365 at its last reading (883 behind; bound 5; 11 other validators answered); anchor disagrees with its peers: block 192 root dc09c513, its peers' dc09c513 (and 79 more)); acc-bvn2-val2 bvn2 (never ACTIVE (BOOTING); executed 956 vs partition 1388 at its last reading (432 behind; bound 5; 3 other validators answered); anchor agreement not measured: it stated no bvn2 anchor after its start); acc-bvn2-val2 directory (never ACTIVE (BOOTING); executed 956 vs partition 1365 at its last reading (409 behind; bound 5; 11 other validators answered); anchor agreement not measured: it stated no directory anchor after its start); acc-bvn3-val1 bvn3 (never ACTIVE (BOOTING); executed 637 vs partition 1404 at its last reading (767 behind; bound 5; 3 other validators answered); anchor agreement not measured: it stated no bvn3 anchor after its start); acc-bvn3-val1 directory (never ACTIVE (BOOTING); executed 618 vs partition 1365 at its last reading (747 behind; bound 5; 11 other validators answered); anchor agreement not measured: it stated no directory anchor after its start); 24 started before the monitor's first sample (the network's launch: not judged) |

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
| blocks the follower stated a root for without sending (#) | 2420 |
| anchored blocks compared, follower vs a validator (#) | 2420 |
| root/BPT mismatches (#) | 16 |
| first mismatching block | Directory block 192: follower root dc09c513 / bpt 463fd311, `acc-bvn1-val1` root dc09c513 / bpt 64dab40f |
| blocks the follower anchored that no validator had (#) | 0 |
| anchor lines carrying no source partition (#4370) (#) | 0 |
| blocks where the follower contradicted itself (#) | 0 |
| certificates refused for a non-committee author (#) | 0 |
| ...of those, authored by the follower (#) | 0 |
| headers dropped by validators for a non-committee author (#) | — not measured (`Header from unknown validator` and `Vote from unknown validator` are slog.Debug with no `module` attribute (vote_handler.go:34,284); the generated logging config sets Debug per module and Info as the default, so this build emits neither line) |
| votes dropped by validators for a non-committee author (#) | — not measured |
| follower behind the validators (blocks, max over the run) | 0, at 2026-09-24T11:19:37Z on BVN3 — the monitor's high-water mark over every tick |
| follower behind the validators (blocks, at the last sample) | BVN3 0, Directory 0 |
| samples where the follower did not answer (#) | 0 |
| accepted, neither certified here, taken on relay, nor refused (#, whole run) | 0, worst 0 on acc-bvn3-fol1/BVN3 (as of 2026-09-24T11:43:36Z; 0 at the last sample; final row written, 0s after the load generator exited; 1 (node, partition) never submitted to this run, counted 0 (acc-bvn3-fol1/Directory); the run was stopped by stallkill, so the load generator was killed mid-flight and this is NOT a drained sample) |
| relayed (#, whole run) | 4750 taken / 0 (series present) refused / 0 (series present) target not ready / 0 (series present) unreachable (as of 2026-09-24T11:43:36Z) |
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
| between the run's start and 11:21Z | crept +0 |
| 11:21Z restart acc-bvn1-val1 | stranded 0 -> 0 (+0) |
| between 11:21Z and 11:24Z | crept +0 |
| 11:24Z pause acc-bvn2-val1 | stranded 0 -> 0 (+0) |
| between 11:24Z and 11:29Z | crept +0 |
| 11:29Z restart acc-bvn3-val1 | stranded 0 -> 0 (+0) |
| between 11:29Z and 11:32Z | crept +0 |
| 11:32Z pause acc-bvn1-val2 | stranded 0 -> 0 (+0) |
| between 11:32Z and 11:36Z | crept +0 |
| 11:36Z restart acc-bvn2-val2 | stranded 0 -> 0 (+0) |
| between 11:36Z and 11:38Z | crept +0 |
| 11:38Z pause acc-bvn3-val2 | stranded 0 -> 0 (+0) (window shortened by the run's start or end) |
| between 11:38Z and the end of the run | crept +0 |
| largest step at a disturbance | +0, at 11:21Z restart acc-bvn1-val1 — no disturbance cost anything |
| largest climb between disturbances | +0, the run's start to 11:21Z — the figure did not climb |

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
