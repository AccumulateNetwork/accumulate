# Ad-hoc capture wedge-20260821T231005Z — block production wedge (#4125)

**Purpose:** preserve what was on disk when a fresh DI network stopped
producing blocks on all four partitions while consensus kept running.

This directory is **not a soak.sh run**. The network was brought up by hand,
so there is no `config/`, no `run.json`, no `soak.log` and no streaming log
capture. What survives is one final soakmon sample and a `docker logs` tail
per node. The limits of that evidence are recorded below because they change
what the capture actually proves.

| field | value |
|---|---|
| captured (UTC) | 2026-08-21T23:10:05Z |
| branch | `issue-4105-collection-proof-delivery` |
| commit | `98d3bf51c` (tree clean apart from this directory) |
| topology | 3 BVNs x 4 validators + bootstrap (13 containers) |
| monitor attached | 2026-08-21T22:49:33Z |
| monitor lifetime | 1232 s |
| heights at capture | Directory 121, BVN1 41, BVN2 29, BVN3 49 |
| loadgen | none (`loadgen: null`) |
| chaos | none |
| metrics scrape | 0 of 0 nodes — monitor ran without container discovery |
| issue | #4125 |

## What the capture shows

Consensus is alive on every node. In the 20 s window covered by
`container-logs/acc-bvn1-val1.log`: 1803 `Received vote via gossip`,
196 `Header handled by primary`, 27 `Created certificate`, and 14
`Bullshark: committing leader chain`. Not one block or execution message,
and **zero** `Failed to process committed certificate` across all 13 nodes.
So `processCommittedCertificate` is not returning an error — it is either
never called or never returns. Leading hypothesis remains that it blocks
collecting a certificate's batches (#4122 territory).

## What the capture does NOT show — corrected 2026-08-22

Two readings taken from this capture on the day do not survive a second look:

1. **"All four partitions stalled at the same instant."** Every partition
   reports `stalledFor: 1231.4`, identical to 0.1 s, and that was read as one
   common cause. It is an artifact. soakmon measures stall from when it first
   saw a height, and it attached at 22:49:33 to a network that was *already*
   wedged — it never observed a single height change. All four counters are
   therefore just the monitor's own uptime. Simultaneity is **not** evidenced
   here. One common cause is still plausible; this capture does not show it.

2. **The onset is absent.** Each node log spans roughly 20 s
   (23:09:45 -> 23:10:05, 3000 lines) because it is a `docker logs` tail taken
   at teardown against bounded container logging. The freeze began at least
   20 minutes earlier. Nothing in this directory can date it.

3. **No goroutine dump was taken** before teardown, which is the one artifact
   that would settle in a single step whether the executor is parked in batch
   collection.

## Harness changes made in response

Gaps 2 and 3 are now closed for any run started by `soak.sh` (see the commit
that follows this one):

- pprof listens on every node, not only bvn1-1
- `wedgewatch.sh` dumps every node's goroutines the moment soakmon reports a
  stalled partition, and keeps the network up afterwards

`soak.sh` already streams `docker compose logs -f` into the run directory from
the first block, so a run started through it cannot lose its onset to rotation.

`container-logs/` is gitignored (13 x ~330 KB); it stays local to this machine.
