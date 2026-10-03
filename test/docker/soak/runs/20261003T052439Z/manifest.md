# Soak run 20261003T052439Z

**Purpose:** follower bootstrap test on lead 78e1463a0

| field | value |
|---|---|
| started (UTC) | 2026-10-03T05:24:39Z |
| commit | `78e1463a090bd62b31aff75e36041bae7a1d297c` |
| describe | `backup/dta-e11-before-lead-sync-553-g78e1463a0` |
| branch | `issue-4205-lead` |
| uncommitted files | 5160: 0 tracked, 5160 untracked (in no patch) |
| image | `disoak-bvn1-val1` |
| image id | `sha256:ba7cdbba0c06ae54d73f6eff768789d1dcf5e42e5266e0232061345348d41b98` |
| late follower image id (acc-bvn3-fol2) | `disoak-bvn3-fol2` `sha256:d97d3e51f9bc2b9da6997cc81c1ba260535a83ed1d4a4989a60bf6aec7681028` (after the build) |
| late follower image id (acc-bvn2-fol1) | `disoak-bvn2-fol1` `sha256:be9839a90f9a1bb260e2c06521e77e66ea49b9a56f5795b9fcec818ee440cebc` (after the build) |
| late follower image id (acc-bvn1-fol1) | `disoak-bvn1-fol1` `sha256:a8cf8a1ce95fe3f1182fba13e083827c4bdc7fc81e488250492504cf703db3e9` (after the build) |
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
| ended (UTC) | 2026-10-03T06:22:08Z |
| elapsed | 0.9h |
| driver exit | 1 (FAILED) |
| Directory height (block, the highest any of its validators that answered executed; first -> last sample) | 40 -> 3280 |
| heals | 0 -> 0 |
| chaos events | 81 |
| monitor samples | 101 |
| seizure | SEIZED at 2026-10-03T05:41:38 :: stalled stream, undelivered for 20 polls :: stuck=n/a stuckStream= worst=BVN1->Directory gap=0 deliv=77 undeliv=anchor Directory->BVN3 undeliv=2 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 1 |
| load generator reads a node would not answer (whole run) | NotReady: 291 answers retried at another endpoint, 0 queries no endpoint would answer; transport error: 0 answers retried at another endpoint, 0 queries failed at every endpoint |
| read-back probe (the validators, whole run) | Whole run: 14350 timed reads, p50 1.7 ms, p95 5.3 ms, p99 45.8 ms, max 200.6 ms (txn read, BVN3, entry 2847 blocks old); 0 failed (0 of them timed out, 8s), 226 refused NotReady (a joining node's designed answer; not timed), 0 refused by the API's query gate (not timed). |
| follower read probe | acc-bvn1-fol1 (partitions Directory, BVN1): 3494 reads of entries it holds, 3276 answered, 218 refused (query gate or NotReady), 0 failed; p50 0.4 ms, p95 2.1 ms, max 201.7 ms. |
| wedge captures (a partition closed no block for 120s; #4125) | 0  |
| delivery-stall captures (every partition kept closing blocks while a synthetic flow into one was red for 120s; #4285) | 0  |
| accepted, neither certified here, taken on relay, nor refused (#, whole run, the validators) | 42, worst 6 on acc-bvn1-val3/BVN1 (as of 2026-10-03T06:22:05Z; rising 40 -> 42 over the last 5 samples; final row written, 0s after the load generator exited) |
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
| blocks the follower stated a root for without sending (#) | 4832 |
| anchored blocks compared, follower vs a validator (#) | 4820 |
| root/BPT mismatches (#) | 4 |
| first mismatching block | BVN3 block 2322: follower root 36c69840 / bpt 43b589bb, `acc-bvn3-fol2` root 36c69840 / bpt efd6c6ca |
| blocks the follower anchored that no validator had (#) | 0 |
| anchor lines carrying no source partition (#4370) (#) | 0 |
| blocks where the follower contradicted itself (#) | 0 |
| certificates refused for a non-committee author (#) | 0 |
| ...of those, authored by the follower (#) | 0 |
| headers dropped by validators for a non-committee author (#) | — not measured (`Header from unknown validator` and `Vote from unknown validator` are slog.Debug with no `module` attribute (vote_handler.go:34,284); the generated logging config sets Debug per module and Info as the default, so this build emits neither line) |
| votes dropped by validators for a non-committee author (#) | — not measured |
| follower behind the validators (blocks, max over the run) | 0, at 2026-10-03T05:27:19Z on BVN3 — the monitor's high-water mark over every tick |
| follower behind the validators (blocks, at the last sample) | BVN3 0, Directory 0 |
| samples where the follower did not answer (#) | 0 |
| accepted, neither certified here, taken on relay, nor refused (#, whole run) | 0, worst 0 on acc-bvn3-fol2/BVN3 (as of 2026-10-03T06:22:05Z; 0 at the last sample; final row written, 0s after the load generator exited; 14 samples skipped as incomplete (a node answered no scrape); 9 counter resets carried forward (0 stranded before a restart; the final row's own readings sum to 0); 4 (node, partition) never submitted to this run, counted 0 (acc-bvn1-fol1/Directory, acc-bvn2-fol1/Directory, acc-bvn3-fol1/Directory, acc-bvn3-fol2/Directory)) |
| relayed (#, whole run) | 4089 taken / 0 (series present) refused / 0 (series present) target not ready / 0 (series present) unreachable (as of 2026-10-03T06:22:05Z) |
| follower start rejoined (per partition, the add-follower and every restart; same reading as the validators' row) | rejoined 21 of 24 start(s) after the launch, worst 114.4s from container start to executing with its partition (acc-bvn1-fol1 directory); NOT rejoined: acc-bvn2-fol1 bvn2 (gauge ACTIVE at 61.7s; anchor disagrees with its peers: block 2334 root add5a17a, its peers' add5a17a (and 4 more)); acc-bvn3-fol2 bvn3 (gauge ACTIVE at 56.0s; anchor disagrees with its peers: block 2322 root 36c69840, its peers' 36c69840 (and 2 more)); acc-bvn3-fol2 bvn3 (gauge ACTIVE at 90.2s; anchor disagrees with its peers: block 2841 root da62e351, its peers' 24aaf1c0); 2 started before the monitor's first sample (the network's launch: not judged) |

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
| 05:33Z remove-follower acc-bvn1-fol1 (removal 1) | not measured (no `follower-removal-1-{before,at,after}.json`) |
| 05:33Z remove-follower acc-bvn2-fol1 (removal 2) | not measured (no `follower-removal-2-{before,at,after}.json`) |
| 05:33Z remove-follower acc-bvn3-fol2 (removal 3) | not measured (no `follower-removal-3-{before,at,after}.json`) |
| 05:33Z add-follower acc-bvn1-fol1 | container start → ACTIVE 9.4s (directory 9.4s); behind at hand-off BVN1 ?, Directory ? blocks (sample 05:33:57Z); first in agreement not measured; NotReady before ACTIVE not measured (no read of it in `readprobe-follower.csv`); accepted 2276, relayed-taken 2276 (100.0% of accepted), stranded 0 (as of 05:56:55Z) |
| 05:33Z add-follower acc-bvn2-fol1 | container start → ACTIVE 8.4s (directory 8.4s); behind at hand-off BVN2 ?, Directory ? blocks (sample 05:33:57Z); first in agreement not measured; NotReady before ACTIVE not measured (no read of it in `readprobe-follower.csv`); accepted 1317, relayed-taken 1317 (100.0% of accepted), stranded 0 (as of 05:56:55Z) |
| 05:33Z add-follower acc-bvn3-fol2 | container start → ACTIVE 8.4s (directory 8.4s); behind at hand-off BVN3 ?, Directory ? blocks (sample 05:33:57Z); first in agreement not measured; NotReady before ACTIVE not measured (no read of it in `readprobe-follower.csv`); accepted 2348, relayed-taken 2348 (100.0% of accepted), stranded 0 (as of 05:56:55Z) |
| 05:57Z remove-follower acc-bvn1-fol1 (removal 4) | not measured (no `follower-removal-4-{before,at,after}.json`) |
| 05:57Z remove-follower acc-bvn2-fol1 (removal 5) | not measured (no `follower-removal-5-{before,at,after}.json`) |
| 05:57Z remove-follower acc-bvn3-fol2 (removal 6) | not measured (no `follower-removal-6-{before,at,after}.json`) |
| 05:57Z add-follower acc-bvn1-fol1 | container start → ACTIVE 56.5s (bvn1 56.5s, directory 56.5s); behind at hand-off BVN1 0, Directory 0 blocks (sample 05:58:03Z); first in agreement not measured; NotReady before ACTIVE on 65 read(s): BVN1 query, Directory query; accepted 771, relayed-taken 771 (100.0% of accepted), stranded 0 (as of 06:04:48Z) |
| 05:57Z add-follower acc-bvn2-fol1 | container start → ACTIVE 55.5s (bvn2 50.2s, directory 55.5s); behind at hand-off BVN2 0, Directory 0 blocks (sample 05:58:03Z); first in agreement not measured; NotReady before ACTIVE on 84 read(s): BVN2 query, Directory query; accepted 435, relayed-taken 435 (100.0% of accepted), stranded 0 (as of 06:04:48Z) |
| 05:57Z add-follower acc-bvn3-fol2 | container start → ACTIVE 67.0s (bvn3 67.0s, directory 55.5s); behind at hand-off BVN3 0, Directory 0 blocks (sample 05:58:03Z); first in agreement not measured; NotReady before ACTIVE on 73 read(s): BVN3 query, Directory query; accepted 789, relayed-taken 789 (100.0% of accepted), stranded 0 (as of 06:04:48Z) |
| 06:05Z remove-follower acc-bvn1-fol1 (removal 7) | not measured (no `follower-removal-7-{before,at,after}.json`) |
| 06:05Z remove-follower acc-bvn2-fol1 (removal 8) | not measured (no `follower-removal-8-{before,at,after}.json`) |
| 06:05Z remove-follower acc-bvn3-fol2 (removal 9) | not measured (no `follower-removal-9-{before,at,after}.json`) |
| 06:05Z add-follower acc-bvn1-fol1 | container start → ACTIVE 61.7s (bvn1 50.8s, directory 61.7s); behind at hand-off BVN1 0, Directory 0 blocks (sample 06:06:18Z); first in agreement not measured; NotReady before ACTIVE on 76 read(s): BVN1 query, Directory query; accepted 754, relayed-taken 754 (100.0% of accepted), stranded 0 (as of 06:13:06Z) |
| 06:05Z add-follower acc-bvn2-fol1 | container start → ACTIVE 61.7s (bvn2 61.7s, directory 61.7s); behind at hand-off BVN2 ?, Directory 0 blocks (sample 06:06:18Z); first in agreement not measured; NotReady before ACTIVE on 73 read(s): BVN2 query, Directory query; accepted 356, relayed-taken 356 (100.0% of accepted), stranded 0 (as of 06:13:06Z) |
| 06:05Z add-follower acc-bvn3-fol2 | container start → ACTIVE 61.7s (bvn3 56.0s, directory 61.7s); behind at hand-off BVN3 0, Directory 0 blocks (sample 06:06:18Z); first in agreement not measured; NotReady before ACTIVE on 75 read(s): BVN3 query, Directory query; accepted 705, relayed-taken 705 (100.0% of accepted), stranded 0 (as of 06:13:06Z) |
| 06:13Z remove-follower acc-bvn1-fol1 (removal 10) | not measured (no `follower-removal-10-{before,at,after}.json`) |
| 06:13Z remove-follower acc-bvn2-fol1 (removal 11) | not measured (no `follower-removal-11-{before,at,after}.json`) |
| 06:13Z remove-follower acc-bvn3-fol2 (removal 12) | not measured (no `follower-removal-12-{before,at,after}.json`) |
| 06:13Z add-follower acc-bvn1-fol1 | container start → ACTIVE 61.9s (bvn1 51.2s, directory 61.9s); behind at hand-off BVN1 0, Directory ? blocks (sample 06:14:30Z); first in agreement not measured; NotReady before ACTIVE on 77 read(s): BVN1 query, Directory query; accepted 864, relayed-taken 864 (100.0% of accepted), stranded 0 (as of 06:22:05Z) |
| 06:13Z add-follower acc-bvn2-fol1 | container start → ACTIVE 78.2s (bvn2 78.2s, directory 61.9s); behind at hand-off BVN2 0, Directory 0 blocks (sample 06:15:00Z); first in agreement not measured; NotReady before ACTIVE on 65 read(s): BVN2 query, Directory query; accepted 525, relayed-taken 525 (100.0% of accepted), stranded 0 (as of 06:22:05Z) |
| 06:13Z add-follower acc-bvn3-fol2 | container start → ACTIVE 90.2s (bvn3 90.2s, directory 61.9s); behind at hand-off BVN3 0, Directory 0 blocks (sample 06:15:00Z); first in agreement not measured; NotReady before ACTIVE on 78 read(s): BVN3 query, Directory query; accepted 837, relayed-taken 837 (100.0% of accepted), stranded 0 (as of 06:22:05Z) |

Full detail in `follower-report.md`; the per-sample series in `follower.csv`.

Raw: `soak.log`, `monitor.csv`, `mem.csv` (every node, with its role), `submissions.csv`, `chaos.log`, `nodestate.csv`, `loadgen-stats.json`, `readprobe.csv` / `readprobe-report.md`, `follower.csv` / `follower-report.md`, `readprobe-follower.csv`, `follower-removal-N-{before,at,after}.json`, `network-definition.json`.

## Disturbance record (chaos.log, copied verbatim; the file itself is gitignored)

```
2026-10-03T05:24:41Z bootstrap-test.py start: HEAD 78e1463a0, 3 cycles, settle 5 min, dwell 5 min
2026-10-03T05:27:21Z network up; harness says commit 78e1463a0 (HEAD 78e1463a0); dashboard answering
2026-10-03T05:27:21Z monitor reachable on the tailnet: http://100.75.75.92:8098/
2026-10-03T05:27:22Z DISABLED for this run (CHAOS=off)
2026-10-03T05:28:51Z load generator running
2026-10-03T05:28:51Z settling 5 min under load before the first cycle
2026-10-03T05:33:51Z remove-follower acc-bvn1-fol1 (bootstrap-test cycle 1)
2026-10-03T05:33:51Z remove-follower acc-bvn2-fol1 (bootstrap-test cycle 1)
2026-10-03T05:33:51Z remove-follower acc-bvn3-fol2 (bootstrap-test cycle 1)
2026-10-03T05:33:52Z add-follower acc-bvn1-fol1 (bootstrap-test cycle 1; databases and consensus/ cleared; join-running-network)
2026-10-03T05:33:52Z add-follower acc-bvn2-fol1 (bootstrap-test cycle 1; databases and consensus/ cleared; join-running-network)
2026-10-03T05:33:52Z add-follower acc-bvn3-fol2 (bootstrap-test cycle 1; databases and consensus/ cleared; join-running-network)
2026-10-03T05:55:41Z bootstrap-test.py STOPPED by hand: cycle 1's followers never got join-running-network (the driver's toml edit failed silently); they replayed from genesis under the #4506 watch instead of joining. Fixing the driver; cycles re-run with --attach.
2026-10-03T05:57:03Z bootstrap-test.py start: HEAD 97b625a84, 3 cycles, settle 0 min, dwell 5 min
2026-10-03T05:57:03Z settling 0 min under load before the first cycle
2026-10-03T05:57:04Z remove-follower acc-bvn1-fol1 (bootstrap-test cycle 1)
2026-10-03T05:57:04Z remove-follower acc-bvn2-fol1 (bootstrap-test cycle 1)
2026-10-03T05:57:04Z remove-follower acc-bvn3-fol2 (bootstrap-test cycle 1)
2026-10-03T05:57:05Z add-follower acc-bvn1-fol1 (bootstrap-test cycle 1; databases and consensus/ cleared; join-running-network)
2026-10-03T05:57:05Z add-follower acc-bvn2-fol1 (bootstrap-test cycle 1; databases and consensus/ cleared; join-running-network)
2026-10-03T05:57:05Z add-follower acc-bvn3-fol2 (bootstrap-test cycle 1; databases and consensus/ cleared; join-running-network)
2026-10-03T05:57:55Z bvn2-fol1/bvn ACTIVE 50s after the add
2026-10-03T05:58:05Z bvn1-fol1/dn.acme ACTIVE 60s after the add
2026-10-03T05:58:05Z bvn1-fol1/bvn ACTIVE 60s after the add
2026-10-03T05:58:05Z bvn2-fol1/dn.acme ACTIVE 60s after the add
2026-10-03T05:58:05Z bvn3-fol2/dn.acme ACTIVE 60s after the add
2026-10-03T05:58:05Z bvn3-fol2/bvn ACTIVE 60s after the add
2026-10-03T05:59:56Z bvn2-fol1/bvn synced 171s after the add (ACTIVE at 50s)
2026-10-03T06:00:06Z bvn1-fol1/dn.acme synced 182s after the add (ACTIVE at 60s)
2026-10-03T06:00:06Z bvn1-fol1/bvn synced 182s after the add (ACTIVE at 60s)
2026-10-03T06:00:06Z bvn2-fol1/dn.acme synced 182s after the add (ACTIVE at 60s)
2026-10-03T06:00:06Z bvn3-fol2/dn.acme synced 182s after the add (ACTIVE at 60s)
2026-10-03T06:00:17Z bvn3-fol2/bvn synced 192s after the add (ACTIVE at 60s)
2026-10-03T06:00:17Z dwell 5 min begins; behind: bvn1-fol1/bvn1=0, bvn1-fol1/directory=0, bvn2-fol1/bvn2=0, bvn2-fol1/directory=0, bvn3-fol2/bvn3=0, bvn3-fol2/directory=0
2026-10-03T06:05:17Z dwell over; drops out of agreement: bvn1-fol1/bvn=0, bvn1-fol1/dn.acme=0, bvn2-fol1/bvn=0, bvn2-fol1/dn.acme=0, bvn3-fol2/bvn=0, bvn3-fol2/dn.acme=0
2026-10-03T06:05:17Z cycle 1 PASSED
2026-10-03T06:05:17Z remove-follower acc-bvn1-fol1 (bootstrap-test cycle 2)
2026-10-03T06:05:17Z remove-follower acc-bvn2-fol1 (bootstrap-test cycle 2)
2026-10-03T06:05:17Z remove-follower acc-bvn3-fol2 (bootstrap-test cycle 2)
2026-10-03T06:05:19Z add-follower acc-bvn1-fol1 (bootstrap-test cycle 2; databases and consensus/ cleared; join-running-network)
2026-10-03T06:05:19Z add-follower acc-bvn2-fol1 (bootstrap-test cycle 2; databases and consensus/ cleared; join-running-network)
2026-10-03T06:05:19Z add-follower acc-bvn3-fol2 (bootstrap-test cycle 2; databases and consensus/ cleared; join-running-network)
2026-10-03T06:06:09Z bvn1-fol1/bvn ACTIVE 50s after the add
2026-10-03T06:06:09Z bvn3-fol2/bvn ACTIVE 50s after the add
2026-10-03T06:06:19Z bvn1-fol1/dn.acme ACTIVE 60s after the add
2026-10-03T06:06:19Z bvn2-fol1/dn.acme ACTIVE 60s after the add
2026-10-03T06:06:19Z bvn2-fol1/bvn ACTIVE 60s after the add
2026-10-03T06:06:19Z bvn3-fol2/dn.acme ACTIVE 60s after the add
2026-10-03T06:08:20Z bvn1-fol1/dn.acme synced 181s after the add (ACTIVE at 60s)
2026-10-03T06:08:20Z bvn1-fol1/bvn synced 181s after the add (ACTIVE at 50s)
2026-10-03T06:08:20Z bvn2-fol1/dn.acme synced 182s after the add (ACTIVE at 60s)
2026-10-03T06:08:20Z bvn3-fol2/dn.acme synced 182s after the add (ACTIVE at 60s)
2026-10-03T06:08:20Z bvn3-fol2/bvn synced 182s after the add (ACTIVE at 50s)
2026-10-03T06:08:30Z bvn2-fol1/bvn synced 192s after the add (ACTIVE at 60s)
2026-10-03T06:08:30Z dwell 5 min begins; behind: bvn1-fol1/bvn1=0, bvn1-fol1/directory=0, bvn2-fol1/bvn2=0, bvn2-fol1/directory=0, bvn3-fol2/bvn3=0, bvn3-fol2/directory=0
2026-10-03T06:13:31Z dwell over; drops out of agreement: bvn1-fol1/bvn=0, bvn1-fol1/dn.acme=0, bvn2-fol1/bvn=0, bvn2-fol1/dn.acme=0, bvn3-fol2/bvn=0, bvn3-fol2/dn.acme=0
2026-10-03T06:13:31Z cycle 2 PASSED
2026-10-03T06:13:31Z remove-follower acc-bvn1-fol1 (bootstrap-test cycle 3)
2026-10-03T06:13:31Z remove-follower acc-bvn2-fol1 (bootstrap-test cycle 3)
2026-10-03T06:13:31Z remove-follower acc-bvn3-fol2 (bootstrap-test cycle 3)
2026-10-03T06:13:32Z add-follower acc-bvn1-fol1 (bootstrap-test cycle 3; databases and consensus/ cleared; join-running-network)
2026-10-03T06:13:32Z add-follower acc-bvn2-fol1 (bootstrap-test cycle 3; databases and consensus/ cleared; join-running-network)
2026-10-03T06:13:32Z add-follower acc-bvn3-fol2 (bootstrap-test cycle 3; databases and consensus/ cleared; join-running-network)
2026-10-03T06:14:23Z bvn1-fol1/bvn ACTIVE 50s after the add
2026-10-03T06:14:33Z bvn1-fol1/dn.acme ACTIVE 61s after the add
2026-10-03T06:14:33Z bvn2-fol1/dn.acme ACTIVE 61s after the add
2026-10-03T06:14:33Z bvn3-fol2/dn.acme ACTIVE 61s after the add
2026-10-03T06:14:43Z bvn2-fol1/bvn ACTIVE 71s after the add
2026-10-03T06:14:53Z bvn3-fol2/bvn ACTIVE 81s after the add
2026-10-03T06:16:24Z bvn1-fol1/bvn synced 172s after the add (ACTIVE at 50s)
2026-10-03T06:16:34Z bvn1-fol1/dn.acme synced 182s after the add (ACTIVE at 61s)
2026-10-03T06:16:34Z bvn2-fol1/dn.acme synced 182s after the add (ACTIVE at 61s)
2026-10-03T06:16:34Z bvn3-fol2/dn.acme synced 182s after the add (ACTIVE at 61s)
2026-10-03T06:16:54Z bvn2-fol1/bvn synced 202s after the add (ACTIVE at 71s)
2026-10-03T06:17:05Z bvn3-fol2/bvn synced 212s after the add (ACTIVE at 81s)
2026-10-03T06:17:05Z dwell 5 min begins; behind: bvn1-fol1/bvn1=0, bvn1-fol1/directory=0, bvn2-fol1/bvn2=0, bvn2-fol1/directory=0, bvn3-fol2/bvn3=0, bvn3-fol2/directory=0
2026-10-03T06:22:05Z dwell over; drops out of agreement: bvn1-fol1/bvn=0, bvn1-fol1/dn.acme=0, bvn2-fol1/bvn=0, bvn2-fol1/dn.acme=0, bvn3-fol2/bvn=0, bvn3-fol2/dn.acme=0
2026-10-03T06:22:05Z cycle 3 PASSED
2026-10-03T06:22:05Z result table written: /home/paul/worktrees/issue-4205-lead/test/docker/soak/runs/20261003T052439Z/bootstrap-test.md
2026-10-03T06:22:05Z bootstrap-test.py PASSED: 3 cycles complete
2026-10-03T06:22:05Z STOP: load generator interrupted (pid 1622955); harness tears down
2026-10-03T06:22:40Z harness exited; network torn down
```
