# Restart test with twelve single restarts, R8-1: run 20261003T232435Z

The run used lead `bb77f9222`, which carries the #4515 release fix (`1b2e27224`), and `restart-test.py` with step E added, at 1 tx/s. The step record is in `chaos.log` and the table is in `restart-test.md`. The run went from 23:24Z on 2026-10-03 to 00:26Z on 2026-10-04.

An earlier launch at 23:12Z (`runs/20261003T231245Z`) stopped at "compose build failed". The harness discards the build output, so the cause is unrecorded. A manual build then succeeded and this run was launched.

## Result: the driver passed every step, but cross-partition delivery stopped in step E (#4517)

The driver checks block production and ACTIVE states. Both held throughout. Cross-partition delivery did not.
- **When.** From about 00:04Z, early in step E, it stalled into all four partitions and never recovered. The dashboard's `worstStall` grew to 1,123 s by the end.
- **Where.** Seven synthetic flows were frozen at teardown, among them BVN2 → BVN3 at 405 received of 499 produced, and BVN2 → Directory at 521 of 583. The anchors from the Directory to BVN1 were 268 short at 00:12:39Z.
- **How it looks.** bvn3-val1 last advanced on BVN2's stream at 00:07:36Z, with `sighted` stuck at 405. The receivers never learned of the later entries, so nothing healed them.
- **When BVN2 restarted.** bvn2-val1 at 00:06:53Z and bvn2-val2 at 00:07:49Z.

So this run is **not** a pass for the network. It is a pass for #4515: no restarted node wedged. A new defect, #4517, sits beside it. The node logs are kept for #4517, not deleted.


| step | what happened |
|---|---|
| A: graceful whole-network restart | Every node was ACTIVE 23 s after the restart, by fresh dashboard rows. The partitions made 295 to 298 blocks in the 5 min after. |
| B: kill whole-network restart | Every node was ACTIVE after 17 s. The partitions made 295 to 297 blocks in 5 min. That is the correct count; the old dwell reported about 1,000 here. |
| C: bvn2-val4's BVN2 store torn | The tear was found 5 s after the start. Every node, bvn2-val4's Directory included, was ACTIVE after 42 s. The partitions made 290 to 292 blocks. Last run this step wedged that Directory for 15 min (#4515). |
| D: two of BVN2's four validators lost, then restored | BVN2 made 0 blocks in 5 min and said it had lost its quorum, while BVN1 and BVN3 made 298 to 299 and the Directory 243. After the restore, both returning Directory nodes were ACTIVE by 23:58:40Z, under 2 min (see "The driver"). All partitions made 288 to 298 blocks in 5 min. |
| E: each of the twelve validators restarted alone, alternating graceful and kill | 12 of 12 had every node ACTIVE, 26 to 91 s after the restart. A separate watcher, following each container's own dashboard rows, agrees: the restarted Directory node was ACTIVE 17 to 87 s after its start. The partitions made 295 to 297 blocks in the 5 min after. |

## What it proves about #4515

**This run.** Fifteen single Directory nodes restarted (C, both D restores and E), and none wedged. No Directory catch-up summary in the logs shows the wedge's signature, which is nothing verified while certificates are buffered.

**Unchanged code.** The wedge struck about 1 in 10 in-process. At that rate 15 clean restarts happen about 1 time in 5, so this run alone is not proof. Together with consim (0 in 45 with the fix, 3 in 29 without), it is good evidence. It is not a 12-hour result.

## Restart burst (for R8-2)

A restarting node still drops votes, because its single event loop is flooded while it catches up. Across the run, "Vote channel full, dropping message" was logged 100 to 500 times per validator, over 3 to 5 restarts each, which is roughly 100 to 200 per restart. Nothing visible came of it: every restart was ACTIVE within 91 s, and votes are rebroadcast. This is the burst the serialized boot (consensus.md, "Restart"; DIFFERENCES C15) would remove.

## The driver

**A hole found during step D.** It passed "every node ACTIVE (0s after the restart)" right after restoring bvn2-val3 and bvn2-val4. Their dashboard rows were missing, and the freshness check looked only at rows that existed. `2480f4099` makes a missing row count as not ACTIVE. It came after this run started, so this run's step D and step E checks had the hole.

**What was checked by hand.** For step D, a foreground watch showed both restored Directory nodes BOOTING at 23:57:40Z and ACTIVE at 23:58:40Z. For step E, the separate watcher followed every restarted container's own rows.

The bulk logs `node-logs-live.txt` (186 MB) and `node-logs.txt` (132 MB) were kept until #4517 was diagnosed, and were deleted on 2026-10-04 after run 20261004T055059Z proved the fix. The findings are on #4517.

Automatic captures:
- `wedge-20261003T235240Z` and `delivery-stall-20261003T235704Z` fall in step D, while BVN2 was deliberately below its quorum. The stall cleared at 23:58Z.
- `delivery-stall-20261004T001239Z` is #4517's.
