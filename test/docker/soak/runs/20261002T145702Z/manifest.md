# Soak run 20261002T145702Z

**Purpose:** fresh network on issue-restart-authored-batches ae94c647b (lead a6893d6ed + #4502 #4503 #4505): redo the whole-network restart and the follower cycles that broke run 20261002T090546Z

| field | value |
|---|---|
| started (UTC) | 2026-10-02T14:57:02Z |
| commit | `d04f77ab7a15fae0533fe9f4c70c3697b6424059` |
| describe | `backup/dta-e11-before-lead-sync-538-gd04f77ab7-dirty` |
| branch | `issue-restart-authored-batches` |
| uncommitted files | 4709: 2 tracked (in `config/uncommitted.patch`), 4707 untracked (in no patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:78049260cf2788a288633b478a27af2852a08cf401562f2f053cef9c1132bf4d` |
| late follower image id (acc-bvn3-fol2) | `disoak-bvn3-fol2` `sha256:45c287097c44e9a1b81d5f757f74638e4c39fbe6cc7e92acc7cf18fde2214208` (after the build) |
| late follower image id (acc-bvn2-fol1) | `disoak-bvn2-fol1` `sha256:b8292d53472aecbb25a71344c97e291eb105010945b0fcb9d00c4c85bbe8a6d6` (after the build) |
| late follower image id (acc-bvn1-fol1) | `disoak-bvn1-fol1` `sha256:9c1f3edd0a25344a119f95ba5f60ad46f63104f929a3c4292ca521b0ec5570d9` (after the build) |
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
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1700MiB (effective, from docker inspect) |

Config as run is frozen in `config/` (soak.conf + override.conf, the compose and network files). Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-10-02T20:52:00Z |
| elapsed | 5.87h |
| driver exit | 1 (FAILED) |
| Directory height (block, the highest any of its validators that answered executed; first -> last sample) | 40 -> 9079 |
| heals | 0 -> 436 |
| chaos events | 24 |
| monitor samples | 647 |
| seizure | SEIZED at 2026-10-02T15:13:42 :: stalled stream, undelivered for 20 polls :: stuck=n/a stuckStream= worst=BVN1->Directory gap=0 deliv=55 undeliv=synthetic BVN2->BVN1 undeliv=2 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 6 |
| load generator reads a node would not answer (whole run) | NotReady: 4951788 answers retried at another endpoint, 443664 queries no endpoint would answer; transport error: 2062247 answers retried at another endpoint, 154044 queries failed at every endpoint |
| read-back probe (the validators, whole run) | Whole run: 74286 timed reads, p50 0.9 ms, p95 23.7 ms, p99 130.5 ms, max 8009.5 ms (chain read, Directory, entry 9019 blocks old); 14660 failed (1 of them timed out, 8s), 28790 refused NotReady (a joining node's designed answer; not timed), 0 refused by the API's query gate (not timed). |
| follower read probe | acc-bvn1-fol1 (partitions Directory, BVN1): 24984 reads of entries it holds, 24640 answered, 268 refused (query gate or NotReady), 76 failed; p50 0.7 ms, p95 13.8 ms, max 6035.7 ms. |
| wedge captures (a partition closed no block for 120s; #4125) | 3 wedge-20261002T172036Z,wedge-20261002T173615Z wedge-20261002T175153Z |
| delivery-stall captures (every partition kept closing blocks while a synthetic flow into one was red for 120s; #4285) | 3 delivery-stall-20261002T161132Z,delivery-stall-20261002T162708Z delivery-stall-20261002T164246Z |
| accepted, neither certified here, taken on relay, nor refused (#, whole run, the validators) | 29605, worst 10199 on acc-bvn2-val3/BVN2 (as of 2026-10-02T17:33:19Z; rising 27796 -> 29605 over the last 5 samples; FINAL ROW MISSING — soakmon's exit write did not land; this reading is mid-drain and up to 30s stale, but 11918s BEFORE the load generator exited — mid-drain; 381 samples skipped as incomplete (a node answered no scrape) — INCLUDING THE LAST, so this is not the final row; 28 counter resets carried forward (15673 stranded before a restart; the final row's own readings sum to 13932); 2 (node, partition) joining at this sample, counted 0 (acc-bvn2-val3/Directory, acc-bvn2-val4/Directory)) |
| restarted node rejoined (per node and partition: gauge ACTIVE, executed block within 5 of the highest block any of the partition's answering validators executed, through its last reading, and every anchor it stated agreeing with its peers'; s = container start to the first sample ACTIVE and within that bound; the validators) | rejoined 16 of 28 start(s) after the launch, worst 13.5s from container start to executing with its partition (acc-bvn1-val4 directory); NOT rejoined: acc-bvn2-val3 bvn2 (gauge ACTIVE at 12.5s; executed 4168 vs partition 8213 at its last reading (4045 behind; bound 5; 3 other validators answered; partition 8215 at the end); anchor disagrees with its peers: block 4154 root 029ecd49, its peers' 68b3395b (and 17 more)); acc-bvn2-val3 bvn2 (never ACTIVE (spine stalled at entry 5179); anchor agreement not measured: it stated no bvn2 anchor after its start); acc-bvn2-val3 directory (never ACTIVE (BOOTING); executed 8261 vs partition 9079 at its last reading (818 behind; bound 5; 8 other validators answered); anchor agreement not measured: it stated no directory anchor after its start); acc-bvn2-val4 bvn2 (gauge ACTIVE at 12.5s; executed 4365 vs partition 8213 at its last reading (3848 behind; bound 5; 3 other validators answered; partition 8215 at the end); anchor disagrees with its peers: block 4154 root 7a3c7bbc, its peers' 68b3395b (and 215 more)); acc-bvn2-val4 bvn2 (never ACTIVE (spine stalled at entry 5179); anchor agreement not measured: it stated no bvn2 anchor after its start); acc-bvn2-val4 directory (never ACTIVE (BOOTING); executed 8261 vs partition 9079 at its last reading (818 behind; bound 5; 8 other validators answered); anchor agreement not measured: it stated no directory anchor after its start); not established: acc-bvn2-val1 bvn2 (gauge ACTIVE at 12.5s; silent since 2026-10-02T17:35:58Z: its last answer, 11759s before the end, was executed 8215; the partition was at 1 at the end); acc-bvn2-val1 directory (gauge ACTIVE at 12.5s; silent since 2026-10-02T17:35:58Z: its last answer, 11759s before the end, was executed 9049; the partition was at 9079 at the end); acc-bvn2-val2 bvn2 (gauge ACTIVE at 12.5s; silent since 2026-10-02T17:35:58Z: its last answer, 11759s before the end, was executed 8215; the partition was at 1 at the end); acc-bvn2-val2 directory (gauge ACTIVE at 12.5s; silent since 2026-10-02T17:35:58Z: its last answer, 11759s before the end, was executed 9049; the partition was at 9079 at the end); acc-bvn3-val2 bvn3 (gauge ACTIVE at 12.5s; silent since 2026-10-02T17:33:38Z: its last answer, 11899s before the end, was executed 9097; the partition was at 17515 at the end); acc-bvn3-val2 directory (gauge ACTIVE at 12.5s; silent since 2026-10-02T17:33:38Z: its last answer, 11899s before the end, was executed 8867; the partition was at 9079 at the end); 24 started before the monitor's first sample (the network's launch: not judged) |

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
| blocks the follower stated a root for without sending (#) | 15934 |
| anchored blocks compared, follower vs a validator (#) | 15917 |
| root/BPT mismatches (#) | 13 |
| first mismatching block | BVN3 block 1988: follower root 9aeabaeb / bpt a8861c2d, `acc-bvn3-fol2` root 3dc36882 / bpt 1dd94b07 |
| blocks the follower anchored that no validator had (#) | 0 |
| anchor lines carrying no source partition (#4370) (#) | 0 |
| blocks where the follower contradicted itself (#) | 0 |
| certificates refused for a non-committee author (#) | 0 |
| ...of those, authored by the follower (#) | 0 |
| headers dropped by validators for a non-committee author (#) | — not measured (`Header from unknown validator` and `Vote from unknown validator` are slog.Debug with no `module` attribute (vote_handler.go:34,284); the generated logging config sets Debug per module and Info as the default, so this build emits neither line) |
| votes dropped by validators for a non-committee author (#) | — not measured |
| follower behind the validators (blocks, max over the run) | 34, at 2026-10-02T17:10:16Z on Directory — the monitor's high-water mark over every tick |
| follower behind the validators (blocks, at the last sample) | BVN3 0, Directory 0 |
| samples where the follower did not answer (#) | 2 |
| accepted, neither certified here, taken on relay, nor refused (#, whole run) | 107076, worst 107076 on acc-bvn2-fol1/BVN2 (as of 2026-10-02T20:51:57Z; rising 106028 -> 107076 over the last 5 samples; final row written, 0s after the load generator exited; 61 samples skipped as incomplete (a node answered no scrape); 10 counter resets carried forward (0 stranded before a restart; the final row's own readings sum to 107076); 4 (node, partition) never submitted to this run, counted 0 (acc-bvn1-fol1/Directory, acc-bvn2-fol1/Directory, acc-bvn3-fol1/Directory, acc-bvn3-fol2/Directory)) |
| relayed (#, whole run) | 17815 taken / 1 refused / 0 (series present) target not ready / 107076 unreachable (as of 2026-10-02T20:51:57Z) |
| follower start rejoined (per partition, the add-follower and every restart; same reading as the validators' row) | rejoined 16 of 26 start(s) after the launch, worst 66.0s from container start to executing with its partition (acc-bvn3-fol2 directory); NOT rejoined: acc-bvn1-fol1 bvn1 (gauge ACTIVE at 66.2s; anchor disagrees with its peers: block 1986 root 70184e02, its peers' 18df1da2 (and 1 more)); acc-bvn1-fol1 bvn1 (gauge ACTIVE at 150.8s; anchor disagrees with its peers: block 2631 root c96802b5, its peers' c96802b5 (and 1 more)); acc-bvn1-fol1 bvn1 (gauge ACTIVE at 113.7s; anchor disagrees with its peers: block 3294 root 944515ba, its peers' 944515ba (and 3 more)); acc-bvn2-fol1 bvn2 (gauge ACTIVE at 60.5s; anchor disagrees with its peers: block 3297 root 07c46e91, its peers' 45bbaf15 (and 3 more)); acc-bvn2-fol1 bvn2 (never ACTIVE (spine stalled at entry 2485)); acc-bvn2-fol1 directory (gauge ACTIVE at 11.5s; executed 7782 vs partition 9079 at its last reading (1297 behind; bound 5; 9 other validators answered)); acc-bvn3-fol2 bvn3 (gauge ACTIVE at 60.0s; anchor disagrees with its peers: block 1988 root 3dc36882, its peers' 9aeabaeb (and 2 more)); acc-bvn3-fol2 bvn3 (gauge ACTIVE at 178.3s; anchor disagrees with its peers: block 2756 root e498dd9e, its peers' 17b3a05c (and 2 more)); acc-bvn3-fol2 bvn3 (gauge ACTIVE at 108.3s; anchor disagrees with its peers: block 3312 root 814d3e6c, its peers' 814d3e6c (and 6 more)); acc-bvn3-fol2 directory (never ACTIVE (spine stalled at entry 3942); executed 4186 vs partition 9079 at its last reading (4893 behind; bound 5; 9 other validators answered)); 2 started before the monitor's first sample (the network's launch: not judged) |

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
| between the run's start and 16:09Z | crept +0 |
| 16:09Z restart complete: | stranded 0 -> 0 (+0) |
| between 16:09Z and the end of the run | crept +106028 |
| largest step at a disturbance | +0, at 16:09Z restart complete: — no disturbance cost anything |
| largest climb between disturbances | +106028, 16:09Z to the end of the run |
| samples dropped as incomplete | 61 of 654 — a node answered no scrape at those (every one of its rows empty; a node empty on one partition only is joining it and counts 0), and a fleet total missing one node dips by that node's count |
| counter resets seen | 10 (acc-bvn1-fol1/BVN1 at 15:43Z carrying 0, acc-bvn2-fol1/BVN2 at 15:43Z carrying 0, acc-bvn3-fol2/BVN3 at 15:43Z carrying 0, acc-bvn1-fol1/BVN1 at 15:54Z carrying 0, acc-bvn2-fol1/BVN2 at 15:54Z carrying 0, acc-bvn3-fol2/BVN3 at 15:54Z carrying 0, and 4 more) — the carry is the floor of the pair's last samples, not its last reading, and the series does not step down |

**Per add-follower and remove-follower (#4364).** Each add is read
over that container's life only, from its `add-follower` line to
its `remove-follower`; a quantity the life did not record reads
`not measured`. A removal is judged on the readings
30s either side of it.

| event | verdict |
|---|---|
| 15:31Z add-follower acc-bvn1-fol1 | container start → ACTIVE 66.2s (bvn1 66.2s, directory 55.8s); behind at hand-off BVN1 ?, Directory ? blocks (sample 15:32:24Z); first in agreement not measured; NO NotReady before ACTIVE (76 read(s), none refused); accepted 1148, relayed-taken 1148 (100.0% of accepted), stranded 0 (as of 15:42:06Z) |
| 15:31Z add-follower acc-bvn3-fol2 | container start → ACTIVE 60.0s (bvn3 60.0s, directory 54.8s); behind at hand-off BVN3 ?, Directory ? blocks (sample 15:32:24Z); first in agreement not measured; NO NotReady before ACTIVE (72 read(s), none refused); accepted 389, relayed-taken 389 (100.0% of accepted), stranded 0 (as of 15:42:06Z) |
| 15:31Z add-follower acc-bvn2-fol1 | container start → ACTIVE 54.8s (bvn2 54.8s, directory 54.8s); behind at hand-off BVN2 ?, Directory ? blocks (sample 15:32:24Z); first in agreement not measured; NO NotReady before ACTIVE (72 read(s), none refused); accepted 1040, relayed-taken 1040 (100.0% of accepted), stranded 0 (as of 15:42:06Z) |
| 15:31Z remove-follower acc-bvn1-fol1 (removal 1) | not measured (no `follower-removal-1-{before,at,after}.json`) |
| 15:31Z remove-follower acc-bvn3-fol2 (removal 2) | not measured (no `follower-removal-2-{before,at,after}.json`) |
| 15:31Z remove-follower acc-bvn2-fol1 (removal 3) | not measured (no `follower-removal-3-{before,at,after}.json`) |
| 15:42Z remove-follower acc-bvn1-fol1 (removal 4) | not measured (no `follower-removal-4-{before,at,after}.json`) |
| 15:42Z remove-follower acc-bvn3-fol2 (removal 5) | not measured (no `follower-removal-5-{before,at,after}.json`) |
| 15:42Z remove-follower acc-bvn2-fol1 (removal 6) | not measured (no `follower-removal-6-{before,at,after}.json`) |
| 15:42Z add-follower acc-bvn1-fol1 | container start → ACTIVE 150.8s (bvn1 150.8s, directory 62.6s); behind at hand-off BVN1 0, Directory 0 blocks (sample 15:45:08Z); first in agreement not measured; NotReady before ACTIVE on 148 read(s): BVN1 query, Directory query; accepted 1142, relayed-taken 1142 (100.0% of accepted), stranded 0 (as of 15:53:12Z) |
| 15:42Z add-follower acc-bvn3-fol2 | container start → ACTIVE 178.3s (bvn3 178.3s, directory 62.6s); behind at hand-off BVN3 0, Directory 0 blocks (sample 15:45:38Z); first in agreement not measured; NotReady before ACTIVE on 156 read(s): BVN3 query, Directory query; accepted 381, relayed-taken 381 (100.0% of accepted), stranded 0 (as of 15:53:12Z) |
| 15:42Z add-follower acc-bvn2-fol1 | container start → ACTIVE 62.6s (bvn2 57.3s, directory 62.6s); behind at hand-off BVN2 ?, Directory 0 blocks (sample 15:43:37Z); first in agreement not measured; NotReady before ACTIVE on 77 read(s): BVN2 query, Directory query; accepted 707, relayed-taken 707 (100.0% of accepted), stranded 0 (as of 15:53:12Z) |
| 15:53Z remove-follower acc-bvn1-fol1 (removal 7) | not measured (no `follower-removal-7-{before,at,after}.json`) |
| 15:53Z remove-follower acc-bvn3-fol2 (removal 8) | not measured (no `follower-removal-8-{before,at,after}.json`) |
| 15:53Z remove-follower acc-bvn2-fol1 (removal 9) | not measured (no `follower-removal-9-{before,at,after}.json`) |
| 15:53Z add-follower acc-bvn1-fol1 | container start → ACTIVE 113.7s (bvn1 113.7s, bvn1 12.5s, directory 60.5s, directory 12.5s); behind at hand-off BVN1 0, Directory 0 blocks (sample 16:09:17Z); first in agreement not measured; NotReady before ACTIVE on 120 read(s): BVN1 query, Directory query; accepted 3065, relayed-taken 3065 (100.0% of accepted), stranded 0 (as of 20:51:57Z) |
| 15:53Z add-follower acc-bvn3-fol2 | container start → ACTIVE 108.3s (bvn3 108.3s, bvn3 11.5s, directory 66.0s); behind at hand-off BVN3 0, Directory ? blocks (sample 16:09:17Z); first in agreement not measured; NotReady before ACTIVE on 97 read(s): BVN3 query, Directory query; accepted 1877, relayed-taken 1877 (100.0% of accepted), stranded 0 (as of 20:51:57Z) |
| 15:53Z add-follower acc-bvn2-fol1 | container start → ACTIVE 60.5s (bvn2 60.5s, directory 60.5s, directory 11.5s); behind at hand-off BVN2 ?, Directory 0 blocks (sample 16:09:17Z); first in agreement not measured; NotReady before ACTIVE on 151 read(s): BVN2 query, Directory query; accepted 119057, relayed-taken 11980 (10.1% of accepted), stranded 107076 (as of 20:51:57Z) |

Full detail in `follower-report.md`; the per-sample series in `follower.csv`.

Raw: `soak.log`, `monitor.csv`, `mem.csv` (every node, with its role), `submissions.csv`, `chaos.log`, `nodestate.csv`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`, `follower.csv` / `follower-report.md`, `readprobe-follower.csv`, `follower-removal-N-{before,at,after}.json`, `network-definition.json`.

## Disturbance record (chaos.log, copied verbatim; the file itself is gitignored)

```
2026-10-02T14:59:27Z DISABLED for this run (CHAOS=off)
2026-10-02T15:31:31Z remove-follower acc-bvn1-fol1 (follower-cycles.sh cycle 1)
2026-10-02T15:31:31Z remove-follower acc-bvn3-fol2 (follower-cycles.sh cycle 1)
2026-10-02T15:31:31Z remove-follower acc-bvn2-fol1 (follower-cycles.sh cycle 1)
2026-10-02T15:31:31Z add-follower acc-bvn1-fol1 (follower-cycles.sh cycle 1; databases and consensus/ cleared; join-running-network)
2026-10-02T15:31:31Z add-follower acc-bvn3-fol2 (follower-cycles.sh cycle 1; databases and consensus/ cleared; join-running-network)
2026-10-02T15:31:31Z add-follower acc-bvn2-fol1 (follower-cycles.sh cycle 1; databases and consensus/ cleared; join-running-network)
2026-10-02T15:42:34Z remove-follower acc-bvn1-fol1 (follower-cycles.sh cycle 2)
2026-10-02T15:42:34Z remove-follower acc-bvn3-fol2 (follower-cycles.sh cycle 2)
2026-10-02T15:42:34Z remove-follower acc-bvn2-fol1 (follower-cycles.sh cycle 2)
2026-10-02T15:42:35Z add-follower acc-bvn1-fol1 (follower-cycles.sh cycle 2; databases and consensus/ cleared; join-running-network)
2026-10-02T15:42:35Z add-follower acc-bvn3-fol2 (follower-cycles.sh cycle 2; databases and consensus/ cleared; join-running-network)
2026-10-02T15:42:35Z add-follower acc-bvn2-fol1 (follower-cycles.sh cycle 2; databases and consensus/ cleared; join-running-network)
2026-10-02T15:53:38Z remove-follower acc-bvn1-fol1 (follower-cycles.sh cycle 3)
2026-10-02T15:53:38Z remove-follower acc-bvn3-fol2 (follower-cycles.sh cycle 3)
2026-10-02T15:53:38Z remove-follower acc-bvn2-fol1 (follower-cycles.sh cycle 3)
2026-10-02T15:53:39Z add-follower acc-bvn1-fol1 (follower-cycles.sh cycle 3; databases and consensus/ cleared; join-running-network)
2026-10-02T15:53:39Z add-follower acc-bvn3-fol2 (follower-cycles.sh cycle 3; databases and consensus/ cleared; join-running-network)
2026-10-02T15:53:39Z add-follower acc-bvn2-fol1 (follower-cycles.sh cycle 3; databases and consensus/ cleared; join-running-network)
2026-10-02T16:09:05Z DELIBERATE WHOLE-NETWORK RESTART: all 12 validators + bvn3-fol1 + the three late followers rm -f'd and recreated at once on their images (build d04f77ab7: #4502 #4503 #4505), harness running; heights before: {'Directory': 4185, 'BVN1': 4151, 'BVN2': 4151, 'BVN3': 4153}
2026-10-02T16:09:08Z restart complete: 16 containers started
2026-10-02T16:48:02Z EVIDENCE: live copies of bvn2-val3's and bvn2-val4's BVN2 stores (torn at the 16:09:05Z kill; BlockchainDB#99, #4507) saved under torn-stores/ (not committed)
2026-10-02T17:18:21Z RECOVERY: acc-bvn2-val3 and acc-bvn2-val4 rm -f'd; their BVN2 stores (/root/.accumulate/bvn2-3/bvnn/data/accumulate.db, bvn2-4/...) and BVN2 consensus checkpoints (consensus/bvn2) cleared, join-running-network set; recreated on the same image so they JOIN BVN2 (torn stores preserved in torn-stores/; #4507, BlockchainDB#99)
2026-10-02T20:51:31Z STOP: load generator interrupted (Paul: stop the run and write it up); harness tears down
```
