# Restart test with twelve single restarts, R9-4: run 20261004T055059Z

The run used lead `a38a66290`, which carries the #4517 fixes R9-1 to R9-3 and the #4515 release fix, and `restart-test.py` at 1 tx/s. Every dwell now also fails on a delivery stall. The step record is in `chaos.log` and the table is in `restart-test.md`. The run went from 05:51Z to 06:47Z on 2026-10-04.

## Result: the network got through every disturbance with all flows delivering

The driver reports FAILED for step D alone, and that failure was the driver's own check (see below).

| step | what happened |
|---|---|
| A: graceful whole-network restart | Every node was ACTIVE 18 s after the restart, by fresh rows. The partitions made 295 to 297 blocks in 5 min, with no delivery stalled. |
| B: kill whole-network restart | Every node was ACTIVE after 18 s. 295 to 298 blocks, no stall. |
| C: bvn2-val4's BVN2 store torn | The tear was found 5 s after the start, and every node was ACTIVE after 27 s. 295 to 296 blocks, no stall. |
| D: two of BVN2's four validators lost, then restored | BVN2 made 0 blocks and said it had lost its quorum, while BVN1 and BVN3 made 298 and the Directory 243. Both restored validators were ACTIVE by 06:25:05Z, about 100 s after the restore. The partitions then made 295 to 298 blocks in 5 min, with **no stall anywhere**. |
| E: each of the twelve validators restarted alone, alternating graceful and kill | 12 of 12 were ACTIVE 20 to 31 s after their restart. The partitions then made 295 to 297 blocks in 5 min, with no stall. |

**Why step D shows FAILED.** Its quorum-loss dwell failed on the flows out of the deliberately stopped BVN2. They cannot deliver while BVN2 stands, so the Directory, BVN1 and BVN3 showed inbound stalls of 293 to 332 s. That was the driver's new delivery check being too strict. `753435139` now excuses a destination whose stalled flows all come from a partition meant to stand. It came after this run started, so it did not apply here.

## Delivery during step E, against run 20261003T232435Z

A foreground watch sampled the dashboard once a minute through step E.

| time | delivery |
|---|---|
| 06:30 to 06:31 | a few synthetic flows lagging 31 to 57 s |
| 06:32 | all flows ok |
| 06:34 | stalls on BVN2 → Directory (5 entries) and BVN2 → BVN3 (3), at 5 and 51 s |
| 06:35 | cleared |
| 06:36 to 06:46 | all flows ok, except one flow lagging 42 s at 06:37 |

In run 20261003T232435Z, the stall that began early in the same step grew to 1,123 s and never cleared: BVN2's Directory anchors stood at 1,980 (#4517). Here every disruption cleared within about a minute.

## What it shows

**#4517.** With anchor streams healed whatever the lag (R9-1), anchors answered from the store (R9-2) and system traffic proposed while lagging (R9-3), single restarts no longer stop cross-partition delivery. Which of the three did the work on Docker is not separated. R9-1 alone is proven in-process by `TestALaggingBVNHealsItsDirectoryAnchorStream`.

**#4515.** Fourteen more single restarts of Directory nodes (C, both D restores, E's twelve) with no wedge. Across the two Docker runs since the fix, that is 29 with none.

**What it does not show.** This was a 56-minute run, and it says nothing about long-run behaviour. That takes the 12-hour acceptance soak.

## Captures

`delivery-stall-20261004T061907Z` and `wedge-20261004T061950Z` fall in step D, while BVN2 was deliberately below its quorum. They are expected and were not read.

The bulk logs `node-logs-live.txt` (196 MB) and `node-logs.txt` (143 MB) are deleted after this summary.
