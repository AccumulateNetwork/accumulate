# Soak run 20261004T172903Z

**Purpose:** #4519 check: follower repair time at 10 tps, followers every ~10 min (lead 62d0594b8)

| field | value |
|---|---|
| started (UTC) | 2026-10-04T17:29:03Z |
| commit | `62d0594b8585f2adf558531bec6252430b0e72d4` |
| describe | `backup/dta-e11-before-lead-sync-594-g62d0594b8` |
| branch | `issue-4205-lead` |
| uncommitted files | 5661: 0 tracked, 5661 untracked (in no patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:77bd4994f415352e34a167e55443e4d772dad2c8947f0b7a46ca267c748658ef` |
| late follower image id (acc-bvn3-fol2) | `disoak-bvn3-fol2` `sha256:11449379c82416f65a4b2a6be3055446208db5c659a3060e134bf6afa931e58c` (after the build) |
| late follower image id (acc-bvn2-fol1) | `disoak-bvn2-fol1` `sha256:1b4c5dfb2f167f232ebbe89fe66d79f1effd6ea4b7ba497bcfbcfd94c9885235` (after the build) |
| late follower image id (acc-bvn1-fol1) | `disoak-bvn1-fol1` `sha256:5ac29cfc093706d1723855020492316f3655f07c3fd2e3ac4d4bc9e4baba205b` (after the build) |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| fault model | add-follower then remove-follower (CHAOS_FOLLOWER_CYCLES=0, 0 unbounded) every 600s + 0-30s |
| topology | 3 BVNs, 12 validators + 4 followers (acc-bvn3-fol1,acc-bvn1-fol1,acc-bvn2-fol1,acc-bvn3-fol2, partitions Directory BVN3;Directory BVN1;Directory BVN2;Directory BVN3), of which the last 3 are started only by the add-follower disturbance + bootstrap |
| follower key | inactive in the definition; active validators per partition: BVN1 4, BVN2 4, BVN3 4, Directory 12 |
| partitions | Directory BVN1 BVN2 BVN3 |
| chaos | on |
| target duration | 1h |
| target TPS | 10 |
| storage | BlockchainDB (docker-network.yml) |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1300MiB (effective, from docker inspect) |

Config as run is frozen in `config/` (soak.conf + override.conf, the compose and network files). Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-10-04T18:32:25Z |
| elapsed | 1.01h |
| driver exit | 0 (clean) |
| Directory height (block, the highest any of its validators that answered executed; first -> last sample) | 40 -> 3650 |
| heals | 0 -> 0 |
| chaos events | 46 |
| monitor samples | 113 |
| seizure | SEIZED at 2026-10-04T17:45:29 :: stalled stream, undelivered for 20 polls :: stuck=n/a stuckStream= worst=BVN1->Directory gap=0 deliv=509 undeliv=synthetic BVN1->Directory undeliv=10 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 2 |
| load generator reads a node would not answer (whole run) | NotReady: 2370 answers retried at another endpoint, 0 queries no endpoint would answer; transport error: 0 answers retried at another endpoint, 0 queries failed at every endpoint |
| read-back probe (the validators, whole run) | Whole run: 16098 timed reads, p50 1.2 ms, p95 9.2 ms, p99 56.6 ms, max 197.4 ms (txn read, BVN1, entry 2350 blocks old); 0 failed (0 of them timed out, 8s), 278 refused NotReady (a joining node's designed answer; not timed), 0 refused by the API's query gate (not timed). |
| follower read probe | acc-bvn1-fol1 (partitions Directory, BVN1): 2135 reads of entries it holds, 1759 answered, 376 refused (query gate or NotReady), 0 failed; p50 0.3 ms, p95 45.9 ms, max 151.1 ms. |
| wedge captures (a partition closed no block for 120s; #4125) | 0  |
| delivery-stall captures (every partition kept closing blocks while a synthetic flow into one was red for 120s; #4285) | 0  |
| accepted, neither certified here, taken on relay, nor refused (#, whole run, the validators) | 55, worst 7 on acc-bvn1-val1/BVN1 (as of 2026-10-04T18:32:22Z; rising 43 -> 55 over the last 5 samples; final row written, 0s after the load generator exited) |
| restarted node rejoined (per node and partition: gauge ACTIVE, executed block within 5 of the highest block any of the partition's answering validators executed, through its last reading, and every anchor it stated agreeing with its peers'; s = container start to the first sample ACTIVE and within that bound; the validators) | no start after the network's launch; 24 started before the monitor's first sample (the network's launch: not judged) |

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
| blocks the follower stated a root for without sending (#) | 7151 |
| anchored blocks compared, follower vs a validator (#) | 7124 |
| root/BPT mismatches (#) | 11 |
| first mismatching block | BVN3 block 2017: follower root 420cbd65 / bpt 3bdfe9e6, `acc-bvn3-fol2` root 5c5c732a / bpt adc9781c |
| blocks the follower anchored that no validator had (#) | 0 |
| anchor lines carrying no source partition (#4370) (#) | 0 |
| blocks where the follower contradicted itself (#) | 0 |
| certificates refused for a non-committee author (#) | 0 |
| ...of those, authored by the follower (#) | 0 |
| headers dropped by validators for a non-committee author (#) | — not measured (`Header from unknown validator` and `Vote from unknown validator` are slog.Debug with no `module` attribute (vote_handler.go:34,284); the generated logging config sets Debug per module and Info as the default, so this build emits neither line) |
| votes dropped by validators for a non-committee author (#) | — not measured |
| follower behind the validators (blocks, max over the run) | 0, at 2026-10-04T17:31:11Z on BVN3 — the monitor's high-water mark over every tick |
| follower behind the validators (blocks, at the last sample) | BVN3 0, Directory 0 |
| samples where the follower did not answer (#) | 0 |
| accepted, neither certified here, taken on relay, nor refused (#, whole run) | 0, worst 0 on acc-bvn3-fol2/BVN3 (as of 2026-10-04T18:32:22Z; 0 at the last sample; final row written, 0s after the load generator exited; 58 samples skipped as incomplete (a node answered no scrape); 6 counter resets carried forward (0 stranded before a restart; the final row's own readings sum to 0); 4 (node, partition) never submitted to this run, counted 0 (acc-bvn1-fol1/Directory, acc-bvn2-fol1/Directory, acc-bvn3-fol1/Directory, acc-bvn3-fol2/Directory)) |
| relayed (#, whole run) | 10433 taken / 0 (series present) refused / 0 (series present) target not ready / 0 (series present) unreachable (as of 2026-10-04T18:32:22Z) |
| follower start rejoined (per partition, the add-follower and every restart; same reading as the validators' row) | rejoined 11 of 18 start(s) after the launch, worst 73.2s from container start to executing with its partition (acc-bvn3-fol2 bvn3); NOT rejoined: acc-bvn1-fol1 bvn1 (gauge ACTIVE at 128.8s; anchor disagrees with its peers: block 1996 root bdb5bb80, its peers' d498abf0 (and 8 more)); acc-bvn1-fol1 bvn1 (gauge ACTIVE at 161.0s; anchor disagrees with its peers: block 3340 root 3b91493d, its peers' 871e7844 (and 10 more)); acc-bvn2-fol1 bvn2 (gauge ACTIVE at 84.9s; anchor disagrees with its peers: block 721 root c5d03699, its peers' 0142830f (and 6 more)); acc-bvn2-fol1 bvn2 (gauge ACTIVE at 128.8s; anchor disagrees with its peers: block 2049 root 21d6db23, its peers' 67b8b5e7 (and 4 more)); acc-bvn2-fol1 bvn2 (gauge ACTIVE at 222.8s; anchor disagrees with its peers: block 3371 root dda8d40a, its peers' 7eee4a62 (and 6 more)); acc-bvn3-fol2 bvn3 (gauge ACTIVE at 84.7s; anchor disagrees with its peers: block 2017 root 5c5c732a, its peers' 420cbd65 (and 4 more)); acc-bvn3-fol2 bvn3 (gauge ACTIVE at 173.1s; anchor disagrees with its peers: block 3340 root 7237295f, its peers' 0ad54859 (and 5 more)); 2 started before the monitor's first sample (the network's launch: not judged) |

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
| stranded across disturbances | — not measured (`chaos.log` records no restart or pause) |

**Per add-follower and remove-follower (#4364).** Each add is read
over that container's life only, from its `add-follower` line to
its `remove-follower`; a quantity the life did not record reads
`not measured`. A removal is judged on the readings
30s either side of it.

| event | verdict |
|---|---|
| 17:41Z add-follower acc-bvn1-fol1 | container start → ACTIVE 44.8s (bvn1 44.8s, directory 44.8s); behind at hand-off BVN1 ?, Directory 0 blocks (sample 17:42:29Z); first in agreement acc://bvn-BVN1.acme block 693 at 2026-10-04T17:42:26Z; acc://dn.acme block 703 at 2026-10-04T17:42:25Z, 52s after the add; NotReady before ACTIVE on 68 read(s): BVN1 query, Directory query; accepted 912, relayed-taken 912 (100.0% of accepted), stranded 0 (as of 17:52:05Z) |
| 17:41Z add-follower acc-bvn2-fol1 | container start → ACTIVE 84.9s (bvn2 84.9s, directory 44.8s); behind at hand-off BVN2 0, Directory 0 blocks (sample 17:42:59Z); first in agreement acc://bvn-BVN2.acme block 719 at 2026-10-04T17:42:51Z; acc://dn.acme block 703 at 2026-10-04T17:42:26Z, 72s after the add; NotReady before ACTIVE on 68 read(s): BVN2 query, Directory query; accepted 1723, relayed-taken 1723 (100.0% of accepted), stranded 0 (as of 17:52:05Z) |
| 17:41Z add-follower acc-bvn3-fol2 | container start → ACTIVE 73.2s (bvn3 73.2s, directory 44.8s); behind at hand-off BVN3 0, Directory 0 blocks (sample 17:42:59Z); first in agreement acc://bvn-BVN3.acme block 710 at 2026-10-04T17:42:42Z; acc://dn.acme block 703 at 2026-10-04T17:42:25Z, 62s after the add; NotReady before ACTIVE on 68 read(s): BVN3 query, Directory query; accepted 1131, relayed-taken 1131 (100.0% of accepted), stranded 0 (as of 17:52:05Z) |
| 17:52Z remove-follower acc-bvn1-fol1, acc-bvn2-fol1, acc-bvn3-fol2 (removal 1) | unaffected: cadence blocks/s before->after: BVN1 0.96->0.99, BVN2 1.00->0.96, BVN3 1.00->0.96, Directory 0.93->0.99; 19 streams, 16 advancing before and after, 3 quiet before |
| 18:03Z add-follower acc-bvn1-fol1 | container start → ACTIVE 128.8s (bvn1 128.8s, directory 68.3s); behind at hand-off BVN1 0, Directory 0 blocks (sample 18:05:22Z); first in agreement acc://bvn-BVN1.acme block 1995 at 2026-10-04T18:04:32Z; acc://dn.acme block 2002 at 2026-10-04T18:04:15Z, 83s after the add; NotReady before ACTIVE on 122 read(s): BVN1 query, Directory query; accepted 889, relayed-taken 889 (100.0% of accepted), stranded 0 (as of 18:13:24Z) |
| 18:03Z add-follower acc-bvn2-fol1 | container start → ACTIVE 128.8s (bvn2 128.8s, directory 68.3s); behind at hand-off BVN2 ?, Directory 0 blocks (sample 18:05:22Z); first in agreement acc://bvn-BVN2.acme block 2045 at 2026-10-04T18:05:19Z; acc://dn.acme block 2002 at 2026-10-04T18:04:15Z, 135s after the add; NotReady before ACTIVE on 106 read(s): BVN2 query, Directory query; accepted 1797, relayed-taken 1797 (100.0% of accepted), stranded 0 (as of 18:13:24Z) |
| 18:03Z add-follower acc-bvn3-fol2 | container start → ACTIVE 84.7s (bvn3 84.7s, directory 68.3s); behind at hand-off BVN3 1, Directory 0 blocks (sample 18:04:50Z); first in agreement acc://bvn-BVN3.acme block 2001 at 2026-10-04T18:04:32Z; acc://dn.acme block 2001 at 2026-10-04T18:04:14Z, 84s after the add; NotReady before ACTIVE on 112 read(s): BVN3 query, Directory query; accepted 1137, relayed-taken 1137 (100.0% of accepted), stranded 0 (as of 18:13:24Z) |
| 18:13Z remove-follower acc-bvn1-fol1, acc-bvn2-fol1, acc-bvn3-fol2 (removal 2) | unaffected: cadence blocks/s before->after: BVN1 0.96->1.00, BVN2 0.96->1.00, BVN3 0.96->1.00, Directory 1.00->1.00; 19 streams, 16 advancing before and after, 3 quiet before |
| 18:24Z add-follower acc-bvn1-fol1 | container start → ACTIVE 161.0s (bvn1 161.0s, directory 71.7s); behind at hand-off BVN1 ?, Directory 0 blocks (sample 18:27:19Z); first in agreement acc://bvn-BVN1.acme block 3338 at 2026-10-04T18:27:20Z; acc://dn.acme block 3285 at 2026-10-04T18:25:50Z, 166s after the add; NotReady before ACTIVE on 106 read(s): BVN1 query, Directory query; accepted 645, relayed-taken 645 (100.0% of accepted), stranded 0 (as of 18:32:22Z) |
| 18:24Z add-follower acc-bvn2-fol1 | container start → ACTIVE 222.8s (bvn2 222.8s, directory 71.7s); behind at hand-off BVN2 ?, Directory 0 blocks (sample 18:28:21Z); first in agreement acc://bvn-BVN2.acme block 3358 at 2026-10-04T18:27:34Z; acc://dn.acme block 3286 at 2026-10-04T18:25:51Z, 176s after the add; NotReady before ACTIVE on 152 read(s): BVN2 query, Directory query; accepted 1283, relayed-taken 1283 (100.0% of accepted), stranded 0 (as of 18:32:22Z) |
| 18:24Z add-follower acc-bvn3-fol2 | container start → ACTIVE 173.1s (bvn3 173.1s, directory 71.7s); behind at hand-off BVN3 0, Directory 0 blocks (sample 18:27:19Z); first in agreement acc://bvn-BVN3.acme block 3338 at 2026-10-04T18:27:10Z; acc://dn.acme block 3283 at 2026-10-04T18:25:48Z, 156s after the add; NotReady before ACTIVE on 114 read(s): BVN3 query, Directory query; accepted 836, relayed-taken 836 (100.0% of accepted), stranded 0 (as of 18:32:22Z) |

Full detail in `follower-report.md`; the per-sample series in `follower.csv`.

Raw: `soak.log`, `monitor.csv`, `mem.csv` (every node, with its role), `submissions.csv`, `chaos.log`, `nodestate.csv`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`, `follower.csv` / `follower-report.md`, `readprobe-follower.csv`, `follower-removal-N-{before,at,after}.json`, `network-definition.json`.
