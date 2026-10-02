# Soak run 20260929T152146Z

**Purpose:** #4466 docker proof: fresh network on lead 03945b6eb; after steady load, one deliberate whole-network restart; every validator must resume and anchors reach quorum

| field | value |
|---|---|
| started (UTC) | 2026-09-29T15:21:46Z |
| commit | `03945b6ebffcc71035b330b0f5c767c4c1e32c2a` |
| describe | `backup/dta-e11-before-lead-sync-437-g03945b6eb-dirty` |
| branch | `issue-4205-lead` |
| uncommitted files | 4284: 1 tracked (in `config/uncommitted.patch`), 4283 untracked (in no patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:2c0af9ae0beb566af0d0682178b31238c7822a885eb79ca1b26f34e823529bbe` |
| late follower image id (acc-bvn3-fol2) | `disoak-bvn3-fol2` `sha256:7f8705126b178fad6af8daa017cb518846c437f73b5993c33f5c88a75df2f46a` (after the build) |
| late follower image id (acc-bvn2-fol1) | `disoak-bvn2-fol1` `sha256:01e36c3d4f21c29c3d9a99950623702d32e720d5bfdf680f81337663b7399378` (after the build) |
| late follower image id (acc-bvn1-fol1) | `disoak-bvn1-fol1` `sha256:0224967b1faa542c9e834be5de33ce70121828207935263381da46f52b445238` (after the build) |
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

## Disturbances (chaos.log, copied 2026-10-01T23:37:18Z; *.log is untracked by decision 456bf4b74)

```
2026-09-29T15:24:19Z DISABLED for this run (CHAOS=off)
2026-09-29T15:41:24Z DELIBERATE WHOLE-NETWORK RESTART (#4466 proof): all 12 validators + bvn3-fol1 recreated at once on the same image (lead 03945b6eb), harness running
2026-09-29T15:53:24Z add-follower acc-bvn1-fol1 (manual, Paul: will a follower bootstrap after the whole restart?; databases cleared; join-running-network)
2026-09-29T21:51:45Z remove-follower acc-bvn1-fol1 (manual; to re-add on lead 9cc09eac0, #4469)
2026-09-29T21:53:26Z add-follower acc-bvn1-fol1 (manual; image 459c09930015 = lead 9cc09eac0 with #4469; validators still 03945b6eb; databases cleared; join-running-network)
2026-09-29T22:50:47Z remove-follower acc-bvn1-fol1 acc-bvn2-fol1 acc-bvn3-fol2 (follower-cycles.sh cycle 1)
2026-09-29T22:50:47Z add-follower acc-bvn1-fol1 acc-bvn2-fol1 acc-bvn3-fol2 (follower-cycles.sh cycle 1; lead 9cc09eac0 images; databases cleared; join-running-network)
2026-09-29T23:00:37Z follower images rebuilt on lead 72b1d43b6 (#4471 backfill gauge); cycles 2+ of follower-cycles.sh use them, cycle 1 runs 9cc09eac0
2026-09-29T23:01:56Z remove-follower acc-bvn1-fol1 acc-bvn2-fol1 acc-bvn3-fol2 (follower-cycles.sh cycle 2)
2026-09-29T23:01:56Z add-follower acc-bvn1-fol1 acc-bvn2-fol1 acc-bvn3-fol2 (follower-cycles.sh cycle 2; lead 9cc09eac0 images; databases cleared; join-running-network)
2026-09-29T23:12:44Z remove-follower acc-bvn1-fol1 acc-bvn2-fol1 acc-bvn3-fol2 (follower-cycles.sh cycle 3)
2026-09-29T23:12:46Z add-follower acc-bvn1-fol1 acc-bvn2-fol1 acc-bvn3-fol2 (follower-cycles.sh cycle 3; lead 9cc09eac0 images; databases cleared; join-running-network)
2026-09-30T01:07:41Z follower images rebuilt on lead be56c8757 (#4472 backfill beside the root watch); follower-cycles.sh cycle 4 uses them
2026-09-30T01:07:46Z remove-follower acc-bvn1-fol1 acc-bvn2-fol1 acc-bvn3-fol2 (follower-cycles.sh cycle 4)
2026-09-30T01:07:47Z add-follower acc-bvn1-fol1 acc-bvn2-fol1 acc-bvn3-fol2 (follower-cycles.sh cycle 4; lead 9cc09eac0 images; databases cleared; join-running-network)
2026-09-30T21:26:33Z follower images rebuilt on lead be8a5cfce (CometBFT/legacy-snapshot removal, #3909/#3910); follower-cycles.sh cycle 5 uses them; validators still 03945b6eb
2026-09-30T21:26:34Z remove-follower acc-bvn1-fol1 acc-bvn2-fol1 acc-bvn3-fol2 (follower-cycles.sh cycle 5)
2026-09-30T21:26:35Z add-follower acc-bvn1-fol1 acc-bvn2-fol1 acc-bvn3-fol2 (follower-cycles.sh cycle 5; lead 9cc09eac0 images; databases cleared; join-running-network)
2026-09-30T21:43:53Z NOTE: the cycle-5 add at 2026-09-30T21:26:33Z was written as one line naming all three followers, and the monitor reads one container per line; recorded again below, one per follower, with the original time
2026-09-30T21:26:33Z add-follower acc-bvn2-fol1 (follower-cycles.sh cycle 5; re-recorded one per line; databases cleared; join-running-network)
2026-09-30T21:26:33Z add-follower acc-bvn3-fol2 (follower-cycles.sh cycle 5; re-recorded one per line; databases cleared; join-running-network)
2026-10-01T21:01:07Z follower images rebuilt on lead 778f1f2d0 (#4483 agreement line); follower-cycles.sh cycle 6 clears consensus/ as well, so the three followers join fresh
2026-10-01T21:01:08Z remove-follower acc-bvn1-fol1 (follower-cycles.sh cycle 6)
2026-10-01T21:01:08Z remove-follower acc-bvn3-fol2 (follower-cycles.sh cycle 6)
2026-10-01T21:01:08Z remove-follower acc-bvn2-fol1 (follower-cycles.sh cycle 6)
2026-10-01T21:01:10Z add-follower acc-bvn1-fol1 (follower-cycles.sh cycle 6; databases and consensus/ cleared; join-running-network)
2026-10-01T21:01:10Z add-follower acc-bvn3-fol2 (follower-cycles.sh cycle 6; databases and consensus/ cleared; join-running-network)
2026-10-01T21:01:10Z add-follower acc-bvn2-fol1 (follower-cycles.sh cycle 6; databases and consensus/ cleared; join-running-network)
```

## Stopped early by stallkill

- stopped (UTC, the decision): 2026-10-02T05:55:23Z
- evidence capture ended, load generator signalled (UTC): 2026-10-02T05:55:56Z (33 s after the decision)
- reason: monitor unreachable for 120s

Evidence was captured from the decision on; see the probe-* directory
written as the capture began. The network ran on, under load and chaos,
until the capture ended.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-10-02T05:53:45Z |
| elapsed | 62.48h |
| driver exit | 1 (FAILED) |
| Directory height (block, the highest any of its validators that answered executed; first -> last sample) | 40 -> 222577 |
| heals | 0 -> 45 |
| chaos events | 29 |
| monitor samples | 6901 |
| seizure | SEIZED at 2026-09-29T15:38:34 :: stalled stream, undelivered for 20 polls :: stuck=n/a stuckStream= worst=BVN1->Directory gap=0 deliv=48 undeliv=anchor BVN3->Directory undeliv=3 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 0 |
| load generator reads a node would not answer (whole run) | NotReady: 17769 answers retried at another endpoint, 75 queries no endpoint would answer; transport error: 1538 answers retried at another endpoint, 140 queries failed at every endpoint |
| read-back probe (the validators, whole run) | Whole run: 1113205 timed reads, p50 2.2 ms, p95 11.1 ms, p99 38.4 ms, max 1095.0 ms (txn read, BVN2, entry 189526 blocks old); 0 failed (0 of them timed out, 8s), 571 refused NotReady (a joining node's designed answer; not timed), 0 refused by the API's query gate (not timed). |
| follower read probe | acc-bvn1-fol1 (partitions Directory, BVN1): 277816 reads of entries it holds, 276580 answered, 1170 refused (query gate or NotReady), 66 failed; p50 0.4 ms, p95 1.2 ms, max 284.8 ms. |
| wedge captures (a partition closed no block for 120s; #4125) | 0  |
| delivery-stall captures (every partition kept closing blocks while a synthetic flow into one was red for 120s; #4285) | 0  |
| accepted, neither certified here, taken on relay, nor refused (#, whole run, the validators) | 65, worst 9 on acc-bvn3-val3/BVN3 (as of 2026-10-02T05:53:36Z; falling 87 -> 65 over the last 5 samples (draining); final row written, 4s after the load generator exited; 24 counter resets carried forward (23 stranded before a restart; the final row's own readings sum to 42); the run was stopped by stallkill, so the load generator was killed mid-flight and this is NOT a drained sample) |
| restarted node rejoined (per node and partition: gauge ACTIVE, executed block within 5 of the highest block any of the partition's answering validators executed, through its last reading, and every anchor it stated agreeing with its peers'; s = container start to the first sample ACTIVE and within that bound; the validators) | rejoined 24 of 24 start(s) after the launch, worst 223929.1s from container start to executing with its partition (acc-bvn3-val4 directory); 24 started before the monitor's first sample (the network's launch: not judged) |

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
| blocks the follower stated a root for without sending (#) | 314411 |
| anchored blocks compared, follower vs a validator (#) | 314411 |
| root/BPT mismatches (#) | 0 |
| first mismatching block | none |
| blocks the follower anchored that no validator had (#) | 0 |
| anchor lines carrying no source partition (#4370) (#) | 0 |
| blocks where the follower contradicted itself (#) | 0 |
| certificates refused for a non-committee author (#) | 0 |
| ...of those, authored by the follower (#) | 0 |
| headers dropped by validators for a non-committee author (#) | — not measured (`Header from unknown validator` and `Vote from unknown validator` are slog.Debug with no `module` attribute (vote_handler.go:34,284); the generated logging config sets Debug per module and Info as the default, so this build emits neither line) |
| votes dropped by validators for a non-committee author (#) | — not measured |
| follower behind the validators (blocks, max over the run) | 1, at 2026-09-29T22:58:29Z on BVN3 — the monitor's high-water mark over every tick |
| follower behind the validators (blocks, at the last sample) | BVN3 0, Directory 0 |
| samples where the follower did not answer (#) | 2 |
| accepted, neither certified here, taken on relay, nor refused (#, whole run) | 0, worst 0 on acc-bvn3-fol2/BVN3 (as of 2026-10-02T05:53:36Z; 0 at the last sample; final row written, 4s after the load generator exited; 782 samples skipped as incomplete (a node answered no scrape); 15 counter resets carried forward (0 stranded before a restart; the final row's own readings sum to 0); 4 (node, partition) never submitted to this run, counted 0 (acc-bvn1-fol1/Directory, acc-bvn2-fol1/Directory, acc-bvn3-fol1/Directory, acc-bvn3-fol2/Directory); the run was stopped by stallkill, so the load generator was killed mid-flight and this is NOT a drained sample) |
| relayed (#, whole run) | 348096 taken / 0 (series present) refused / 0 (series present) target not ready / 0 (series present) unreachable (as of 2026-10-02T05:53:36Z) |
