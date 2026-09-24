# Soak run 20260924T052134Z

**Purpose:** 30m 100tps chaos on issue-4205-lead 2d969f9e2: first Docker run of the second-pass join (#4362 a-d, #4362f, #4356 merged). Every restarted validator must rejoin: anchor body equal to its peers on both partitions, Directory 12 of 12. Launched by the interactive session, Paul watching via 100.75.75.92:8098.

| field | value |
|---|---|
| started (UTC) | 2026-09-24T05:21:34Z |
| commit | `2d969f9e2db2631e10974fc6f9ef122edcc1e540` |
| describe | `backup/dta-e11-before-lead-sync-80-g2d969f9e2` |
| branch | `issue-4205-lead` |
| uncommitted files | 0 |
| image | `disoak-bvn1-val1` |
| image id | `sha256:c8b0a42187ee266e296ebd4d42deb61a5599cfb0bac3749fb63105702edb7ff6` |
| late follower image id (acc-bvn3-fol2) | `disoak-bvn3-fol2` `sha256:f26451ff1c995792a2d6c485774367adc461238d9d20878f516319b8b8cb2fc8` (after the build) |
| executor version | **v2-kourou** |
| healing | unconditional (DI conductor, #4105) |
| fault model | restart or pause one BVN validator container every 120s + 0-60s |
| topology | 3 BVNs, 12 validators + 2 follower (acc-bvn3-fol1,acc-bvn3-fol2, partitions Directory BVN3;Directory BVN3) + bootstrap |
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

- stopped (UTC): 2026-09-24T05:47:00Z
- reason: stalled 245s: BVN1 (threshold 240s)

Evidence was captured before stopping; see the probe-* directory
written at that moment.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-09-24T05:47:01Z |
| elapsed | 0.39h |
| driver exit | 143 (FAILED) |
| dn height | 39 -> 1362 |
| heals | 0 -> 10019 |
| chaos events | 15 |
| monitor samples | 44 |
| seizure | SEIZED at 2026-09-24T05:26:47 :: stuck=n/a stuckStream= worst=BVN2->BVN1 gap=242 deliv=2201 undeliv=synthetic BVN2->BVN3 undeliv=121 |
| reconcile pulls (#4073) | 0 |
| stalled channels at end | 9 |
| read-back probe | Whole run: 4880 timed reads, p50 1.8 ms, p95 8.0 ms, p99 30.6 ms, max 8013.3 ms (txn read, BVN2, entry 381 blocks old); 795 failed, 28 timed out (8s), 0 refused by the API's query gate (not timed). |
| follower read probe | acc-bvn3-fol1 (partitions Directory, BVN3): 1206 reads of entries it holds, 1206 answered, 0 refused (query gate or NotReady), 0 failed; p50 0.5 ms, p95 1.4 ms, max 7.8 ms. |
| wedge captures (#4125) | 1 wedge-20260924T053825Z |
| accepted, neither certified here, taken on relay, nor refused (#, whole run, the validators) | 5001, worst 4153 on acc-bvn1-val1/BVN1 (as of 2026-09-24T05:47:00Z; rising 3274 -> 5001 over the last 5 samples; final row written, 0s after the load generator exited; 14 samples skipped as incomplete (a node reported no counts); 6 counter resets carried forward; the run was stopped by stallkill, so the load generator was killed mid-flight and this is NOT a drained sample) |
| container start → ACTIVE (s, per node and partition, every start the monitor saw; the validators) | worst 29.4s (acc-bvn2-val2 directory) over 4 start(s) seen booting; 24 ACTIVE at first sight (started before the monitor saw them: upper bounds, not in the worst); NEVER ACTIVE: acc-bvn2-val2 bvn2 (BOOTING), acc-bvn3-val1 bvn3 (BOOTING) |

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
| blocks the follower stated a root for without sending (#) | 2413 |
| anchored blocks compared, follower vs a validator (#) | 2413 |
| root/BPT mismatches (#) | 0 |
| first mismatching block | none |
| blocks the follower anchored that no validator had (#) | 0 |
| anchor lines carrying no source partition (#4370) (#) | 0 |
| blocks where the follower contradicted itself (#) | 0 |
| certificates refused for a non-committee author (#) | 0 |
| ...of those, authored by the follower (#) | 0 |
| headers dropped by validators for a non-committee author (#) | — not measured (`Header from unknown validator` and `Vote from unknown validator` are slog.Debug with no `module` attribute (vote_handler.go:34,284); the generated logging config sets Debug per module and Info as the default, so this build emits neither line) |
| votes dropped by validators for a non-committee author (#) | — not measured |
| follower behind the validators (blocks, max over the run) | 1, at 2026-09-24T05:30:57Z on BVN3 — the monitor's high-water mark over every tick |
| follower behind the validators (blocks, at the last sample) | BVN3 0, Directory 0 |
| samples where the follower did not answer (#) | 80 |
| accepted, neither certified here, taken on relay, nor refused (#, whole run) | — not measured (no sample has all 2 (node, partition) pairs; 36 incomplete) |
| relayed (#, whole run) | 3394 taken / 0 (series present) refused / 0 (series present) target not ready / 0 (series present) unreachable (as of 2026-09-24T05:47:00Z) |
| container start → ACTIVE (s, per partition, the add-follower and every restart) | no start inside the run was seen booting; 2 ACTIVE at first sight (started before the monitor saw them: upper bounds, not in the worst); every start reached ACTIVE |

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
| stranded across disturbances | — not measured (no sample has all 2 (node, partition) pairs; 36 incomplete) |

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
