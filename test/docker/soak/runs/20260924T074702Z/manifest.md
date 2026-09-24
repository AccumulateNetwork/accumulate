# Soak run 20260924T074702Z

**Purpose:** 30m 100tps chaos on issue-4205-lead 3a02ec41d: second Docker run of the second-pass join, with every mechanism the first run (20260924T052134Z) exposed fixed and reviewed: #4397/#4399, #4398/#4401/#4402, #4400, #4407, #4362e, #4363, #4395, #4404. Every restarted validator must rejoin on both partitions, Directory 12 of 12. Relaunch after a transient compose build failure at 07:30Z. Launched by the interactive session, Paul watching via 100.75.75.92:8098.

| field | value |
|---|---|
| started (UTC) | 2026-09-24T07:47:02Z |
| commit | `3a02ec41d1b7f40524dc96ed09ff9564c1e66f1f` |
| describe | `backup/dta-e11-before-lead-sync-193-g3a02ec41d` |
| branch | `issue-4205-lead` |
| uncommitted files | 100: 0 tracked, 100 untracked (in no patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:bdfd64ae9dd94fd9ea47fcc793c9ce91674d64b7e41fb86b8340ddcb378674af` |
| late follower image id (acc-bvn3-fol2) | `disoak-bvn3-fol2` `sha256:0a92c5950c9dee186b2f0335af73e346bb4769f58634e45313de0242e5c9fbec` (after the build) |
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

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-24T08:21:02Z |
| elapsed | 0.53h |
| driver exit | 0 (clean) |
| Directory height (block, the highest any of its validators that answered executed; first -> last sample) | 40 -> 1841 |
| heals | 0 -> 9199 |
| chaos events | 20 |
| monitor samples | 60 |
| seizure | SEIZED at 2026-09-24T07:52:20 :: stuck=n/a stuckStream= worst=BVN1->Directory gap=61 deliv=168 undeliv=synthetic BVN2->BVN1 undeliv=143 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 9 |
| load generator reads a node would not answer (whole run) | NotReady: 44231 answers retried at another endpoint, 0 queries no endpoint would answer; transport error: 872 answers retried at another endpoint, 0 queries failed at every endpoint |
| read-back probe | Whole run: 7152 timed reads, p50 1.2 ms, p95 4.5 ms, p99 15.1 ms, max 8009.5 ms (txn read, BVN2, entry 408 blocks old); 906 failed, 40 timed out (8s), 0 refused by the API's query gate (not timed). |
| follower read probe | acc-bvn3-fol1 (partitions Directory, BVN3): 1799 reads of entries it holds, 1799 answered, 0 refused (query gate or NotReady), 0 failed; p50 0.4 ms, p95 0.7 ms, max 12.1 ms. |
| wedge captures (#4125) | 1 wedge-20260924T081419Z |
| accepted, neither certified here, taken on relay, nor refused (#, whole run, the validators) | 1413, worst 909 on acc-bvn1-val1/Directory (as of 2026-09-24T08:05:42Z; rising 1009 -> 1413 over the last 5 samples; FINAL ROW MISSING — soakmon's exit write did not land; this reading is mid-drain and up to 30s stale, but 918s BEFORE the load generator exited — mid-drain; 26 samples skipped as incomplete (a node reported no counts) — INCLUDING THE LAST, so this is not the final row; 4 counter resets carried forward) |
| restarted node rejoined (per node and partition: gauge ACTIVE, executed block within 5 of the highest block any of the partition's answering validators executed, through its last reading, and every anchor it stated agreeing with its peers'; s = container start to the first sample ACTIVE and within that bound; the validators) | rejoined 2 of 10 start(s) after the launch, worst 16.4s from container start to executing with its partition (acc-bvn3-val1 bvn3); NOT rejoined: acc-bvn1-val1 directory (gauge ACTIVE at 10.0s; executed 184 vs partition 1867 at its last reading (1683 behind; bound 5; 11 other validators answered); never ACTIVE and within 5 blocks of the partition at one sample; anchor agreement not measured: it stated no directory anchor after its start); acc-bvn1-val3 bvn1 (never ACTIVE (BOOTING); executed 1375 vs partition 1923 at its last reading (548 behind; bound 5; 3 other validators answered); anchor agreement not measured: it stated no bvn1 anchor after its start); acc-bvn1-val3 directory (gauge ACTIVE at 83.4s; executed 1350 vs partition 1867 at its last reading (517 behind; bound 5; 11 other validators answered); never ACTIVE and within 5 blocks of the partition at one sample; anchor agreement not measured: it stated no directory anchor after its start); acc-bvn2-val2 bvn2 (never ACTIVE (BOOTING); executed 1030 vs partition 1869 at its last reading (839 behind; bound 5; 3 other validators answered); anchor agreement not measured: it stated no bvn2 anchor after its start); acc-bvn2-val2 directory (gauge ACTIVE at 13.0s; executed 1043 vs partition 1867 at its last reading (824 behind; bound 5; 11 other validators answered); never ACTIVE and within 5 blocks of the partition at one sample; anchor agreement not measured: it stated no directory anchor after its start); acc-bvn3-val1 directory (gauge ACTIVE at 10.7s; executed 661 vs partition 1867 at its last reading (1206 behind; bound 5; 11 other validators answered); anchor disagrees with its peers: block 658 root 9c5794e5, its peers' 99db2d11 (and 14 more)); acc-bvn3-val3 bvn3 (never ACTIVE (BOOTING); executed 1805 vs partition 1926 at its last reading (121 behind; bound 5; 3 other validators answered); anchor agreement not measured: it stated no bvn3 anchor after its start); acc-bvn3-val3 directory (never ACTIVE (BOOTING); executed 1745 vs partition 1867 at its last reading (122 behind; bound 5; 11 other validators answered); anchor agreement not measured: it stated no directory anchor after its start); 24 started before the monitor's first sample (the network's launch: not judged) |

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
| blocks the follower stated a root for without sending (#) | 3278 |
| anchored blocks compared, follower vs a validator (#) | 3278 |
| root/BPT mismatches (#) | 0 |
| first mismatching block | none |
| blocks the follower anchored that no validator had (#) | 0 |
| anchor lines carrying no source partition (#4370) (#) | 0 |
| blocks where the follower contradicted itself (#) | 0 |
| certificates refused for a non-committee author (#) | 0 |
| ...of those, authored by the follower (#) | 0 |
| headers dropped by validators for a non-committee author (#) | — not measured (`Header from unknown validator` and `Vote from unknown validator` are slog.Debug with no `module` attribute (vote_handler.go:34,284); the generated logging config sets Debug per module and Info as the default, so this build emits neither line) |
| votes dropped by validators for a non-committee author (#) | — not measured |
| follower behind the validators (blocks, max over the run) | 0, at 2026-09-24T07:48:32Z on BVN3 — the monitor's high-water mark over every tick |
| follower behind the validators (blocks, at the last sample) | BVN3 0, Directory 0 |
| samples where the follower did not answer (#) | 0 |
| accepted, neither certified here, taken on relay, nor refused (#, whole run) | — not measured (no sample has all 2 (node, partition) pairs; 49 incomplete) |
| relayed (#, whole run) | 6664 taken / 2 refused / 0 (series present) target not ready / 0 (series present) unreachable (as of 2026-09-24T08:21:00Z) |
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
| stranded across disturbances | — not measured (no sample has all 2 (node, partition) pairs; 49 incomplete) |

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
