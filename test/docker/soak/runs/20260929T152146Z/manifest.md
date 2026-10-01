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
