# Soak run 20261002T090546Z

**Purpose:** fresh network on lead dfd1efcf8: sectioned BPT sync, redirection, backfill rework, overlap with Pauls

| field | value |
|---|---|
| started (UTC) | 2026-10-02T09:05:46Z |
| commit | `dfd1efcf84f7150ddc5d2e1e85de5f0e50226e07` |
| describe | `backup/dta-e11-before-lead-sync-530-gdfd1efcf8` |
| branch | `issue-4205-lead` |
| uncommitted files | 4333: 0 tracked, 4333 untracked (in no patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:78049260cf2788a288633b478a27af2852a08cf401562f2f053cef9c1132bf4d` |
| late follower image id (acc-bvn3-fol2) | `disoak-bvn3-fol2` `sha256:8a0a6c7543203bb804337756d85982a5bbe47ee25d7f398094ec0bcd918edba2` (after the build) |
| late follower image id (acc-bvn2-fol1) | `disoak-bvn2-fol1` `sha256:63e5f80d1b848975197f906368888c0a1070de0ec78022549250421200ffe1b2` (after the build) |
| late follower image id (acc-bvn1-fol1) | `disoak-bvn1-fol1` `sha256:44046c0a4f176e8b1ec651d19555dc2178bdbf90c0efd04f92e40061f9ccff7e` (after the build) |
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
| ended (UTC) | 2026-10-02T14:53:39Z |
| elapsed | 5.74h |
| driver exit | 1 (FAILED) |
| Directory height (block, the highest any of its validators that answered executed; first -> last sample) | 40 -> 19823 |
| heals | 0 -> 2 |
| chaos events | 29 |
| monitor samples | 638 |
| seizure | SEIZED at 2026-10-02T09:22:47 :: stalled stream, undelivered for 20 polls :: stuck=n/a stuckStream= worst=BVN1->Directory gap=0 deliv=86 undeliv=synthetic BVN1->Directory undeliv=2 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 2 |
| load generator reads a node would not answer (whole run) | NotReady: 58813 answers retried at another endpoint, 112 queries no endpoint would answer; transport error: 6669 answers retried at another endpoint, 557 queries failed at every endpoint |
| read-back probe (the validators, whole run) | Whole run: 90596 timed reads, p50 1.2 ms, p95 6.8 ms, p99 51.5 ms, max 3151.0 ms (txn read, Directory, entry 13992 blocks old); 318 failed (0 of them timed out, 8s), 10380 refused NotReady (a joining node's designed answer; not timed), 0 refused by the API's query gate (not timed). |
| follower read probe | acc-bvn1-fol1 (partitions Directory, BVN1): 12305 reads of entries it holds, 7072 answered, 3970 refused (query gate or NotReady), 1263 failed; p50 0.4 ms, p95 31.1 ms, max 1267.3 ms. |
| wedge captures (a partition closed no block for 120s; #4125) | 3 wedge-20261002T124928Z,wedge-20261002T130504Z wedge-20261002T132040Z |
| delivery-stall captures (every partition kept closing blocks while a synthetic flow into one was red for 120s; #4285) | 0  |
| accepted, neither certified here, taken on relay, nor refused (#, whole run, the validators) | 56411, worst 19304 on acc-bvn3-val1/BVN3 (as of 2026-10-02T14:53:35Z; rising 55538 -> 56411 over the last 5 samples; final row written, 0s after the load generator exited; 2 samples skipped as incomplete (a node answered no scrape); 34 counter resets carried forward (12627 stranded before a restart; the final row's own readings sum to 43784); 4 (node, partition) joining at this sample, counted 0 (acc-bvn3-val1/Directory, acc-bvn3-val2/Directory, acc-bvn3-val3/Directory, acc-bvn3-val4/Directory)) |
| restarted node rejoined (per node and partition: gauge ACTIVE, executed block within 5 of the highest block any of the partition's answering validators executed, through its last reading, and every anchor it stated agreeing with its peers'; s = container start to the first sample ACTIVE and within that bound; the validators) | rejoined 20 of 33 start(s) after the launch, worst 13.0s from container start to executing with its partition (acc-bvn3-val3 directory); NOT rejoined: acc-bvn3-val1 directory (never ACTIVE (BOOTING); executed 14144 vs partition 19829 at its last reading (5685 behind; bound 5; 11 other validators answered); anchor agreement not measured: it stated no directory anchor after its start); acc-bvn3-val2 directory (never ACTIVE (BOOTING); executed 14144 vs partition 19829 at its last reading (5685 behind; bound 5; 11 other validators answered); anchor agreement not measured: it stated no directory anchor after its start); acc-bvn3-val3 bvn3 (never ACTIVE (spine stalled at entry 6292); anchor agreement not measured: it stated no bvn3 anchor after its start); acc-bvn3-val3 directory (never ACTIVE (spine stalled at entry 18586); executed 14144 vs partition 19829 at its last reading (5685 behind; bound 5; 11 other validators answered); anchor agreement not measured: it stated no directory anchor after its start); acc-bvn3-val4 directory (never ACTIVE (BOOTING); executed 14144 vs partition 19829 at its last reading (5685 behind; bound 5; 11 other validators answered); anchor agreement not measured: it stated no directory anchor after its start); not established: acc-bvn3-val1 bvn3 (gauge ACTIVE at 11.0s; anchor agreement not measured: it stated no bvn3 anchor after its start); acc-bvn3-val1 bvn3 (gauge ACTIVE at 14.7s; anchor agreement not measured: it stated no bvn3 anchor after its start); acc-bvn3-val2 bvn3 (gauge ACTIVE at 12.0s; anchor agreement not measured: it stated no bvn3 anchor after its start); acc-bvn3-val2 bvn3 (gauge ACTIVE at 14.7s; anchor agreement not measured: it stated no bvn3 anchor after its start); acc-bvn3-val3 bvn3 (gauge ACTIVE at 13.0s; anchor agreement not measured: it stated no bvn3 anchor after its start); acc-bvn3-val3 bvn3 (gauge ACTIVE at 14.7s; anchor agreement not measured: it stated no bvn3 anchor after its start); acc-bvn3-val4 bvn3 (gauge ACTIVE at 12.0s; anchor agreement not measured: it stated no bvn3 anchor after its start); acc-bvn3-val4 bvn3 (gauge ACTIVE at ≤9.8s (boot time not measured: ACTIVE at the monitor's first sight of this start); anchor agreement not measured: it stated no bvn3 anchor after its start); superseded: acc-bvn3-val3 bvn3 (restarted again at 2026-10-02T13:17:29Z, before it was ACTIVE); acc-bvn3-val3 directory (restarted again at 2026-10-02T13:15:35Z, before it was ACTIVE); acc-bvn3-val3 directory (restarted again at 2026-10-02T13:17:24Z, before it was ACTIVE); 24 started before the monitor's first sample (the network's launch: not judged) |

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
| blocks the follower stated a root for without sending (#) | 24901 |
| anchored blocks compared, follower vs a validator (#) | 24884 |
| root/BPT mismatches (#) | 9 |
| first mismatching block | BVN3 block 11479: follower root 6c0f053b / bpt 5822c763, `acc-bvn3-fol2` root 13ae5078 / bpt 6e44cadc |
| blocks the follower anchored that no validator had (#) | 0 |
| anchor lines carrying no source partition (#4370) (#) | 0 |
| blocks where the follower contradicted itself (#) | 0 |
| certificates refused for a non-committee author (#) | 0 |
| ...of those, authored by the follower (#) | 0 |
| headers dropped by validators for a non-committee author (#) | 1727 |
| votes dropped by validators for a non-committee author (#) | 8448 |
| follower behind the validators (blocks, max over the run) | 0, at 2026-10-02T09:08:29Z on BVN3 — the monitor's high-water mark over every tick |
| follower behind the validators (blocks, at the last sample) | BVN3 0, Directory 0 |
| samples where the follower did not answer (#) | 0 |
| accepted, neither certified here, taken on relay, nor refused (#, whole run) | 193, worst 97 on acc-bvn3-fol2/BVN3 (as of 2026-10-02T14:36:58Z; flat at 193 -> 193 over the last 5 samples; FINAL ROW MISSING — soakmon's exit write did not land; this reading is mid-drain and up to 30s stale, but 997s BEFORE the load generator exited — mid-drain; 356 samples skipped as incomplete (a node answered no scrape) — INCLUDING THE LAST, so this is not the final row; 12 counter resets carried forward (14 stranded before a restart; the final row's own readings sum to 179); 4 (node, partition) never submitted to this run, counted 0 (acc-bvn1-fol1/Directory, acc-bvn2-fol1/Directory, acc-bvn3-fol1/Directory, acc-bvn3-fol2/Directory)) |
| relayed (#, whole run) | 20788 taken / 0 (series present) refused / 0 (series present) target not ready / 179 unreachable (as of 2026-10-02T14:53:35Z) |

## Disturbance record (chaos.log, copied verbatim; the file itself is gitignored)

```
2026-10-02T09:08:32Z DISABLED for this run (CHAOS=off)
2026-10-02T12:10:14Z remove-follower acc-bvn1-fol1 (follower-cycles.sh cycle 1)
2026-10-02T12:10:14Z remove-follower acc-bvn3-fol2 (follower-cycles.sh cycle 1)
2026-10-02T12:10:14Z remove-follower acc-bvn2-fol1 (follower-cycles.sh cycle 1)
2026-10-02T12:10:15Z add-follower acc-bvn1-fol1 (follower-cycles.sh cycle 1; databases and consensus/ cleared; join-running-network)
2026-10-02T12:10:15Z add-follower acc-bvn3-fol2 (follower-cycles.sh cycle 1; databases and consensus/ cleared; join-running-network)
2026-10-02T12:10:15Z add-follower acc-bvn2-fol1 (follower-cycles.sh cycle 1; databases and consensus/ cleared; join-running-network)
2026-10-02T12:21:37Z follower images rebuilt on lead 9c3219730 (#4495: overlap ends after 20 perfect blocks with every section loaded); cycle 2 uses them, validators unchanged at dfd1efcf8
2026-10-02T12:21:38Z remove-follower acc-bvn1-fol1 (follower-cycles.sh cycle 2)
2026-10-02T12:21:38Z remove-follower acc-bvn3-fol2 (follower-cycles.sh cycle 2)
2026-10-02T12:21:38Z remove-follower acc-bvn2-fol1 (follower-cycles.sh cycle 2)
2026-10-02T12:21:39Z add-follower acc-bvn1-fol1 (follower-cycles.sh cycle 2; databases and consensus/ cleared; join-running-network)
2026-10-02T12:21:39Z add-follower acc-bvn3-fol2 (follower-cycles.sh cycle 2; databases and consensus/ cleared; join-running-network)
2026-10-02T12:21:39Z add-follower acc-bvn2-fol1 (follower-cycles.sh cycle 2; databases and consensus/ cleared; join-running-network)
2026-10-02T12:37:47Z follower images rebuilt on lead a6893d6ed (#4495: 20 perfect blocks with every section loaded, then synced only after a 100-block hold with no regression); cycle 3 uses them; dashboard restarted on the new follower panel
2026-10-02T12:37:48Z remove-follower acc-bvn1-fol1 (follower-cycles.sh cycle 3)
2026-10-02T12:37:48Z remove-follower acc-bvn3-fol2 (follower-cycles.sh cycle 3)
2026-10-02T12:37:48Z remove-follower acc-bvn2-fol1 (follower-cycles.sh cycle 3)
2026-10-02T12:37:49Z add-follower acc-bvn1-fol1 (follower-cycles.sh cycle 3; databases and consensus/ cleared; join-running-network)
2026-10-02T12:37:49Z add-follower acc-bvn3-fol2 (follower-cycles.sh cycle 3; databases and consensus/ cleared; join-running-network)
2026-10-02T12:37:49Z add-follower acc-bvn2-fol1 (follower-cycles.sh cycle 3; databases and consensus/ cleared; join-running-network)
2026-10-02T12:47:06Z DELIBERATE WHOLE-NETWORK RESTART: all 12 validators + bvn3-fol1 + the three late followers recreated at once on their images (validators dfd1efcf8, late followers a6893d6ed), harness running
2026-10-02T13:05:53Z DIAGNOSTIC RESTART: BVN3's 4 validators recreated on diag-restart-stalls 3f1705364 (stall line names consensus round state; disproved proofs logged) to reproduce BVN3's round-25839 stall from its checkpoint
2026-10-02T13:15:34Z DIAGNOSTIC: acc-bvn3-val3 restarted with the default log rule at DEBUG (accumulate.toml in the volume; backup accumulate.toml.pre-debug) to read the vote gates for round 25839
2026-10-02T13:17:21Z DIAGNOSTIC: acc-bvn3-val3 config restored (default rule back to WARN) and restarted; debug showed round 25839's two unvotable headers each name one batch no node holds
2026-10-02T13:54:54Z DIAGNOSTIC: acc-bvn1-fol1 recreated on diag-restart-stalls 3f1705364 (= lead a6893d6ed + logging), databases and consensus/ KEPT, to log which Directory block the disproved BVN2->BVN1 proof for entry 1896 names; its earlier logs are node-logs-acc-bvn1-fol1-before-diag.txt
2026-10-02T14:05:43Z DIAGNOSTIC: acc-bvn1-fol1 recreated on diag-restart-stalls 8ff2d9947 (adds: a missing terminal anchor is looked for in the Directory root chain by scan), databases KEPT
2026-10-02T14:37:28Z DIAGNOSTIC: acc-bvn1-fol1 stopped (docker stop) to copy its BVN1 store for an offline read of the Directory root chain's element-index records
2026-10-02T14:53:35Z STOP: load generator interrupted (Paul: reset the network and redo the tests on the fix branch); harness tears down
```

## Stopped early by stallkill

- stopped (UTC, the decision): 2026-10-02T14:55:25Z
- evidence capture ended, load generator signalled (UTC): 2026-10-02T14:55:58Z (33 s after the decision)
- reason: monitor unreachable for 120s

Evidence was captured from the decision on; see the probe-* directory
written as the capture began. The network ran on, under load and chaos,
until the capture ended.
| follower start rejoined (per partition, the add-follower and every restart; same reading as the validators' row) | rejoined 16 of 28 start(s) after the launch, worst 776.7s from container start to executing with its partition (acc-bvn3-fol2 directory); NOT rejoined: acc-bvn1-fol1 bvn1 (gauge ACTIVE at 49.0s; anchor disagrees with its peers: block 10824 root c11fb69a, its peers' 5ee6139b (and 1 more)); acc-bvn1-fol1 bvn1 (gauge ACTIVE at 59.4s; anchor disagrees with its peers: block 11501 root 0603ecea, its peers' 0603ecea (and 3 more)); acc-bvn1-fol1 bvn1 (gauge ACTIVE at 293.5s; anchor disagrees with its peers: block 12477 root 6f2587ac, its peers' cf46e1f2 (and 7 more)); acc-bvn1-fol1 bvn1 (never ACTIVE (BOOTING); silent since 2026-10-02T14:37:21Z: its last answer, 974s before the end, was executed 16894; the partition was at 20316 at the end; anchor agreement not measured: it stated no bvn1 anchor after its start); acc-bvn1-fol1 directory (never ACTIVE (BOOTING); silent since 2026-10-02T14:37:21Z: its last answer, 974s before the end, was executed 16733; the partition was at 19829 at the end; anchor agreement not measured: it stated no directory anchor after its start); acc-bvn2-fol1 bvn2 (gauge ACTIVE at 169.5s; anchor disagrees with its peers: block 10762 root 98d3fdd7, its peers' 98d3fdd7 (and 3 more)); acc-bvn2-fol1 bvn2 (gauge ACTIVE at 53.2s; anchor disagrees with its peers: block 11425 root d7734095, its peers' d7734095 (and 4 more)); acc-bvn2-fol1 bvn2 (gauge ACTIVE at 67.2s; anchor disagrees with its peers: block 12388 root f9fa422e, its peers' c40057bb); acc-bvn3-fol2 bvn3 (gauge ACTIVE at 250.4s; anchor disagrees with its peers: block 11479 root 13ae5078, its peers' 6c0f053b (and 2 more)); acc-bvn3-fol2 bvn3 (gauge ACTIVE at 358.8s; anchor disagrees with its peers: block 12436 root 780a4353, its peers' afdce049 (and 5 more)); not established: acc-bvn3-fol1 bvn3 (gauge ACTIVE at 13.0s; anchor agreement not measured: it stated no bvn3 anchor after its start); acc-bvn3-fol2 bvn3 (gauge ACTIVE at 10.0s; anchor agreement not measured: it stated no bvn3 anchor after its start); superseded: acc-bvn1-fol1 bvn1 (restarted again at 2026-10-02T14:05:48Z, before it was ACTIVE); acc-bvn1-fol1 directory (restarted again at 2026-10-02T14:05:48Z, before it was ACTIVE); 2 started before the monitor's first sample (the network's launch: not judged) |

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
| stranded across disturbances | chaos off — no disturbances to attribute steps to |

**Per add-follower and remove-follower (#4364).** Each add is read
over that container's life only, from its `add-follower` line to
its `remove-follower`; a quantity the life did not record reads
`not measured`. A removal is judged on the readings
30s either side of it.

| event | verdict |
|---|---|
| 12:10Z remove-follower acc-bvn1-fol1 (removal 1) | not measured (no `follower-removal-1-{before,at,after}.json`) |
| 12:10Z remove-follower acc-bvn3-fol2 (removal 2) | not measured (no `follower-removal-2-{before,at,after}.json`) |
| 12:10Z remove-follower acc-bvn2-fol1 (removal 3) | not measured (no `follower-removal-3-{before,at,after}.json`) |
| 12:10Z add-follower acc-bvn1-fol1 | container start → ACTIVE 60.0s (bvn1 49.0s, directory 60.0s); behind at hand-off BVN1 1, Directory ? blocks (sample 12:11:03Z); first in agreement not measured; NotReady before ACTIVE on 73 read(s): BVN1 query, Directory query; accepted 942, relayed-taken 942 (100.0% of accepted), stranded 0 (as of 12:21:28Z) |
| 12:10Z add-follower acc-bvn3-fol2 | container start → ACTIVE 65.3s (bvn3 60.0s, directory 65.3s); behind at hand-off BVN3 ?, Directory 0 blocks (sample 12:11:34Z); first in agreement not measured; NotReady before ACTIVE on 76 read(s): BVN3 query, Directory query; accepted 411, relayed-taken 411 (100.0% of accepted), stranded 0 (as of 12:21:28Z) |
| 12:10Z add-follower acc-bvn2-fol1 | container start → ACTIVE 169.5s (bvn2 169.5s, directory 65.3s); behind at hand-off BVN2 ?, Directory 0 blocks (sample 12:13:07Z); first in agreement not measured; NotReady before ACTIVE on 156 read(s): BVN2 query, Directory query; accepted 765, relayed-taken 765 (100.0% of accepted), stranded 0 (as of 12:21:28Z) |
| 12:21Z remove-follower acc-bvn1-fol1 (removal 4) | not measured (no `follower-removal-4-{before,at,after}.json`) |
| 12:21Z remove-follower acc-bvn3-fol2 (removal 5) | not measured (no `follower-removal-5-{before,at,after}.json`) |
| 12:21Z remove-follower acc-bvn2-fol1 (removal 6) | not measured (no `follower-removal-6-{before,at,after}.json`) |
| 12:21Z add-follower acc-bvn1-fol1 | container start → ACTIVE 59.4s (bvn1 59.4s, directory 59.4s); behind at hand-off BVN1 1, Directory 0 blocks (sample 12:22:43Z); first in agreement not measured; NotReady before ACTIVE on 79 read(s): BVN1 query, Directory query; accepted 1201, relayed-taken 1201 (100.0% of accepted), stranded 0 (as of 12:37:19Z) |
| 12:21Z add-follower acc-bvn3-fol2 | container start → ACTIVE 250.4s (bvn3 250.4s, directory 59.4s); behind at hand-off BVN3 0, Directory 0 blocks (sample 12:25:46Z); first in agreement not measured; NotReady before ACTIVE on 141 read(s): BVN3 query, Directory query; accepted 550, relayed-taken 550 (100.0% of accepted), stranded 0 (as of 12:37:19Z) |
| 12:21Z add-follower acc-bvn2-fol1 | container start → ACTIVE 59.4s (bvn2 53.2s, directory 59.4s); behind at hand-off BVN2 ?, Directory 0 blocks (sample 12:22:43Z); first in agreement not measured; NotReady before ACTIVE on 79 read(s): BVN2 query, Directory query; accepted 697, relayed-taken 697 (100.0% of accepted), stranded 0 (as of 12:37:19Z) |
| 12:37Z remove-follower acc-bvn1-fol1 (removal 7) | not measured (no `follower-removal-7-{before,at,after}.json`) |
| 12:37Z remove-follower acc-bvn3-fol2 (removal 8) | not measured (no `follower-removal-8-{before,at,after}.json`) |
| 12:37Z remove-follower acc-bvn2-fol1 (removal 9) | not measured (no `follower-removal-9-{before,at,after}.json`) |
| 12:37Z add-follower acc-bvn1-fol1 | container start → ACTIVE 293.5s (bvn1 293.5s, bvn1 12.0s, directory 67.2s, directory 12.0s); behind at hand-off BVN1 0, Directory 0 blocks (sample 12:47:29Z); first in agreement not measured; NotReady before ACTIVE on 308 read(s): BVN1 query, Directory query; accepted 1607, relayed-taken 1607 (100.0% of accepted), stranded 0 (as of 14:36:58Z) |
| 12:37Z add-follower acc-bvn3-fol2 | container start → ACTIVE 358.8s (bvn3 358.8s, bvn3 10.0s, directory 67.2s, directory 10.0s); behind at hand-off BVN3 0, Directory 0 blocks (sample 12:47:29Z); first in agreement not measured; NotReady before ACTIVE on 261 read(s): BVN3 query, Directory query; accepted 3613, relayed-taken 3516 (97.3% of accepted), stranded 97 (as of 14:53:35Z) |
| 12:37Z add-follower acc-bvn2-fol1 | container start → ACTIVE 67.2s (bvn2 67.2s, bvn2 11.0s, directory 67.2s, directory 11.0s); behind at hand-off BVN2 0, Directory 0 blocks (sample 12:47:29Z); first in agreement not measured; NotReady before ACTIVE on 197 read(s): BVN2 query, Directory query; accepted 4648, relayed-taken 4648 (100.0% of accepted), stranded 0 (as of 14:53:35Z) |

Full detail in `follower-report.md`; the per-sample series in `follower.csv`.

Raw: `soak.log`, `monitor.csv`, `mem.csv` (every node, with its role), `submissions.csv`, `chaos.log`, `nodestate.csv`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`, `follower.csv` / `follower-report.md`, `readprobe-follower.csv`, `follower-removal-N-{before,at,after}.json`, `network-definition.json`.
