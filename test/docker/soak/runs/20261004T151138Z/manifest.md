# Soak run 20261004T151138Z

**Purpose:** 12h, 10 tps, late followers added and removed every ~15 min (lead 61868eb40)

| field | value |
|---|---|
| started (UTC) | 2026-10-04T15:11:38Z |
| commit | `61868eb40a837ecd341fb918bad6bab4e35353e9` |
| describe | `backup/dta-e11-before-lead-sync-587-g61868eb40` |
| branch | `issue-4205-lead` |
| uncommitted files | 5658: 0 tracked, 5658 untracked (in no patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:77bd4994f415352e34a167e55443e4d772dad2c8947f0b7a46ca267c748658ef` |
| late follower image id (acc-bvn3-fol2) | `disoak-bvn3-fol2` `sha256:0a525a853bd06e696c2c05c6a87f36c459fcf8c0862d06dceb3b05e780e62b41` (after the build) |
| late follower image id (acc-bvn2-fol1) | `disoak-bvn2-fol1` `sha256:5351e333655241e85bd15375a0fdcc735d728989fca7b4ff3cc2e846d7311980` (after the build) |
| late follower image id (acc-bvn1-fol1) | `disoak-bvn1-fol1` `sha256:f8f2ccca55c365b4da23bd857cccc0bf489834d4119b8569dcbe59c35b805aff` (after the build) |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| fault model | add-follower then remove-follower (CHAOS_FOLLOWER_CYCLES=0, 0 unbounded) every 900s + 0-60s |
| topology | 3 BVNs, 12 validators + 4 followers (acc-bvn3-fol1,acc-bvn1-fol1,acc-bvn2-fol1,acc-bvn3-fol2, partitions Directory BVN3;Directory BVN1;Directory BVN2;Directory BVN3), of which the last 3 are started only by the add-follower disturbance + bootstrap |
| follower key | inactive in the definition; active validators per partition: BVN1 4, BVN2 4, BVN3 4, Directory 12 |
| partitions | Directory BVN1 BVN2 BVN3 |
| chaos | on |
| target duration | 12h |
| target TPS | 10 |
| storage | BlockchainDB (docker-network.yml) |
| block interval | 1s |
| memory budget | mem_limit 2048MiB, GOMEMLIMIT 1300MiB (effective, from docker inspect) |

Config as run is frozen in `config/` (soak.conf + override.conf, the compose and network files). Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-10-04T15:50:37Z |
| elapsed | 0.6h |
| driver exit | 1 (FAILED) |
| Directory height (block, the highest any of its validators that answered executed; first -> last sample) | 40 -> 2194 |
| heals | 0 -> 0 |
| chaos events | 22 |
| monitor samples | 68 |
| seizure | SEIZED at 2026-10-04T15:28:19 :: stalled stream, undelivered for 20 polls :: stuck=n/a stuckStream= worst=BVN1->Directory gap=0 deliv=397 undeliv=synthetic BVN2->BVN1 undeliv=12 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 9 |
| load generator reads a node would not answer (whole run) | NotReady: 2672 answers retried at another endpoint, 0 queries no endpoint would answer; transport error: 0 answers retried at another endpoint, 0 queries failed at every endpoint |
| read-back probe (the validators, whole run) | Whole run: 8612 timed reads, p50 1.2 ms, p95 3.0 ms, p99 25.1 ms, max 117.7 ms (txn read, BVN2, entry 1337 blocks old); 0 failed (0 of them timed out, 8s), 564 refused NotReady (a joining node's designed answer; not timed), 0 refused by the API's query gate (not timed). |
| follower read probe | acc-bvn1-fol1 (partitions Directory, BVN1): 1183 reads of entries it holds, 714 answered, 392 refused (query gate or NotReady), 77 failed; p50 0.3 ms, p95 54.1 ms, max 168.8 ms. |
| wedge captures (a partition closed no block for 120s; #4125) | 0  |
| delivery-stall captures (every partition kept closing blocks while a synthetic flow into one was red for 120s; #4285) | 0  |
| accepted, neither certified here, taken on relay, nor refused (#, whole run, the validators) | 50, worst 6 on acc-bvn1-val2/BVN1 (as of 2026-10-04T15:50:36Z; falling 70 -> 50 over the last 5 samples (draining); final row written, 0s after the load generator exited) |
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
| blocks the follower stated a root for without sending (#) | 4308 |
| anchored blocks compared, follower vs a validator (#) | 4299 |
| root/BPT mismatches (#) | 18 |
| first mismatching block | BVN3 block 1000: follower root eb0bce7a / bpt a7d3d571, `acc-bvn3-fol2` root 478976e7 / bpt 282c62eb |
| blocks the follower anchored that no validator had (#) | 0 |
| anchor lines carrying no source partition (#4370) (#) | 0 |
| blocks where the follower contradicted itself (#) | 0 |
| certificates refused for a non-committee author (#) | 0 |
| ...of those, authored by the follower (#) | 0 |
| headers dropped by validators for a non-committee author (#) | — not measured (`Header from unknown validator` and `Vote from unknown validator` are slog.Debug with no `module` attribute (vote_handler.go:34,284); the generated logging config sets Debug per module and Info as the default, so this build emits neither line) |
| votes dropped by validators for a non-committee author (#) | — not measured |
| follower behind the validators (blocks, max over the run) | 0, at 2026-10-04T15:14:01Z on BVN3 — the monitor's high-water mark over every tick |
| follower behind the validators (blocks, at the last sample) | BVN3 0, Directory 0 |
| samples where the follower did not answer (#) | 0 |
| accepted, neither certified here, taken on relay, nor refused (#, whole run) | 0, worst 0 on acc-bvn3-fol2/BVN3 (as of 2026-10-04T15:45:03Z; 0 at the last sample; FINAL ROW MISSING — soakmon's exit write did not land; this reading is mid-drain and up to 30s stale, but 333s BEFORE the load generator exited — mid-drain; 37 samples skipped as incomplete (a node answered no scrape) — INCLUDING THE LAST, so this is not the final row; 4 (node, partition) never submitted to this run, counted 0 (acc-bvn1-fol1/Directory, acc-bvn2-fol1/Directory, acc-bvn3-fol1/Directory, acc-bvn3-fol2/Directory)) |
| relayed (#, whole run) | 6794 taken / 0 (series present) refused / 0 (series present) target not ready / 0 (series present) unreachable (as of 2026-10-04T15:50:36Z) |
| follower start rejoined (per partition, the add-follower and every restart; same reading as the validators' row) | rejoined 0 of 6 start(s) after the launch; NOT rejoined: acc-bvn1-fol1 bvn1 (gauge ACTIVE at 682.9s; anchor disagrees with its peers: block 978 root a83a1246, its peers' 8083c50a; silent since 2026-10-04T15:45:09Z: its last answer, 327s before the end, was executed 1874; the partition was at 2193 at the end); acc-bvn2-fol1 bvn2 (gauge ACTIVE at 862.3s; anchor disagrees with its peers: block 994 root 7dcac3fe, its peers' 1ddad7e1 (and 2 more); silent since 2026-10-04T15:45:09Z: its last answer, 327s before the end, was executed 1866; the partition was at 2183 at the end); acc-bvn3-fol2 bvn3 (gauge ACTIVE at 77.9s; anchor disagrees with its peers: block 1000 root 478976e7, its peers' eb0bce7a (and 17 more); silent since 2026-10-04T15:45:09Z: its last answer, 327s before the end, was executed 1875; the partition was at 2194 at the end); not established: acc-bvn1-fol1 directory (gauge ACTIVE at 54.6s; silent since 2026-10-04T15:45:09Z: its last answer, 327s before the end, was executed 1887; the partition was at 2208 at the end); acc-bvn2-fol1 directory (gauge ACTIVE at 54.6s; silent since 2026-10-04T15:45:09Z: its last answer, 327s before the end, was executed 1887; the partition was at 2208 at the end); acc-bvn3-fol2 directory (gauge ACTIVE at 54.6s; silent since 2026-10-04T15:45:09Z: its last answer, 327s before the end, was executed 1887; the partition was at 2208 at the end); 2 started before the monitor's first sample (the network's launch: not judged) |

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
| 15:29Z add-follower acc-bvn1-fol1 | container start → ACTIVE 682.9s (bvn1 682.9s, directory 54.6s); behind at hand-off BVN1 0, Directory 0 blocks (sample 15:40:33Z); first in agreement acc://bvn-BVN1.acme block 974 at 2026-10-04T15:29:58Z; acc://dn.acme block 980 at 2026-10-04T15:29:56Z, 52s after the add; NotReady before ACTIVE on 392 read(s): BVN1 query; accepted 749, relayed-taken 749 (100.0% of accepted), stranded 0 (as of 15:45:03Z) |
| 15:29Z add-follower acc-bvn2-fol1 | container start → ACTIVE 862.3s (bvn2 862.3s, directory 54.6s); behind at hand-off BVN2 0, Directory 0 blocks (sample 15:43:38Z); first in agreement acc://bvn-BVN2.acme block 992 at 2026-10-04T15:30:22Z; acc://dn.acme block 981 at 2026-10-04T15:29:56Z, 83s after the add; NotReady before ACTIVE on 549 read(s): BVN2 query; accepted 2164, relayed-taken 2164 (100.0% of accepted), stranded 0 (as of 15:45:03Z) |
| 15:29Z add-follower acc-bvn3-fol2 | container start → ACTIVE 77.9s (bvn3 77.9s, directory 54.6s); behind at hand-off BVN3 ?, Directory 0 blocks (sample 15:30:18Z); first in agreement acc://bvn-BVN3.acme block 999 at 2026-10-04T15:30:24Z; acc://dn.acme block 981 at 2026-10-04T15:29:56Z, 83s after the add; NotReady before ACTIVE on 36 read(s): BVN3 query; accepted 909, relayed-taken 909 (100.0% of accepted), stranded 0 (as of 15:45:03Z) |
| 15:45Z remove-follower acc-bvn1-fol1, acc-bvn2-fol1, acc-bvn3-fol2 (removal 1) | unaffected: cadence blocks/s before->after: BVN1 1.00->0.99, BVN2 1.00->0.96, BVN3 1.00->0.99, Directory 1.00->0.99; 19 streams, 16 advancing before and after, 3 quiet before |

Full detail in `follower-report.md`; the per-sample series in `follower.csv`.

Raw: `soak.log`, `monitor.csv`, `mem.csv` (every node, with its role), `submissions.csv`, `chaos.log`, `nodestate.csv`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`, `follower.csv` / `follower-report.md`, `readprobe-follower.csv`, `follower-removal-N-{before,at,after}.json`, `network-definition.json`.
