# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 3275 timed reads, p50 1.0 ms, p95 4.1 ms, p99 8007.4 ms, **max 8008.6 ms** (txn read, BVN2, entry 329 blocks old); 36 failed (36 of them timed out, 8s), 1179 refused NotReady (a joining node's designed answer; not timed), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 40 | 1.6 | 3.1 | 5.8 |
| 100–1000 | 2340 | 1.0 | 3.6 | 8008.6 |
| 1000–5000 | 895 | 1.0 | 5.1 | 8008.2 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 1878 | 0.7 | 2.0 | 8008.2 |
| txn | 1397 | 1.9 | 6.5 | 8008.6 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 8008.6 | txn | BVN2 | 329 |
| 8008.4 | txn | BVN3 | 340 |
| 8008.3 | txn | BVN1 | 336 |
| 8008.2 | chain | Directory | 1150 |
| 8008.2 | chain | BVN3 | 1199 |
| 8008.2 | txn | Directory | 426 |
| 8008.1 | chain | Directory | 1150 |
| 8008.1 | txn | BVN1 | 336 |
| 8008.1 | chain | BVN2 | 1153 |
| 8008.1 | txn | BVN1 | 336 |

## Followers (#4365) — does a node in no committee answer?

**acc-bvn3-fol1** (partitions Directory, BVN3): 1116 reads of entries it holds, **1116 answered**, 0 refused (query gate or NotReady), 0 failed; p50 0.4 ms, p95 1.1 ms, max 10.2 ms.

Read from its own port, never through the router: a read that another node could have answered says nothing about this one. Entries of partitions it does not run are not asked for.

**acc-bvn3-fol2**: declared (late-follower profile) and not running at any probe round — not a follower of this run; not probed.


## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | NotReady | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|---|
| 11:19:40 | 8 | 2.1 | 5.8 | 5.8 | txn Directory age 37 | 0 | 0 | 0 | 0 | 37 |
| 11:20:40 | 32 | 1.5 | 2.1 | 2.2 | txn BVN3 age 95 | 0 | 0 | 0 | 0 | 99 |
| 11:21:40 | 56 | 1.3 | 2.5 | 4.1 | chain BVN1 age 154 | 0 | 0 | 0 | 0 | 158 |
| 11:22:40 | 80 | 0.7 | 2.0 | 2.6 | txn BVN3 age 214 | 0 | 14 | 0 | 0 | 218 |
| 11:23:40 | 104 | 1.0 | 6.4 | 11.7 | txn BVN3 age 273 | 0 | 17 | 0 | 0 | 277 |
| 11:26:24 | 128 | 1.6 | 8008.0 | 8008.6 | txn BVN2 age 329 | 12 | 18 | 0 | 12 | 342 |
| 11:27:13 | 136 | 1.6 | 6933.5 | 8008.2 | txn Directory age 426 | 5 | 18 | 0 | 5 | 436 |
| 11:27:26 | 144 | 1.4 | 3.3 | 12.5 | txn BVN3 age 495 | 0 | 17 | 0 | 0 | 495 |
| 11:28:26 | 168 | 0.7 | 1.7 | 2.1 | txn BVN1 age 550 | 0 | 25 | 0 | 0 | 555 |
| 11:29:26 | 192 | 0.8 | 2.1 | 4.1 | txn BVN2 age 560 | 0 | 28 | 0 | 0 | 614 |
| 11:30:27 | 216 | 1.0 | 4.4 | 23.1 | txn BVN3 age 614 | 0 | 53 | 0 | 0 | 669 |
| 11:31:27 | 240 | 1.0 | 3.2 | 7.2 | txn BVN3 age 730 | 0 | 61 | 0 | 0 | 730 |
| 11:32:28 | 264 | 0.8 | 3.6 | 23.2 | txn BVN2 age 738 | 0 | 64 | 0 | 0 | 788 |
| 11:33:59 | 286 | 1.6 | 5.4 | 8007.9 | txn BVN2 age 798 | 3 | 66 | 0 | 3 | 849 |
| 11:34:28 | 300 | 0.9 | 5.0 | 10.4 | txn BVN2 age 857 | 0 | 78 | 0 | 0 | 908 |
| 11:35:29 | 300 | 0.8 | 2.0 | 5.4 | txn BVN2 age 916 | 0 | 74 | 0 | 0 | 967 |
| 11:36:29 | 300 | 0.8 | 2.3 | 17.7 | txn BVN2 age 974 | 0 | 105 | 0 | 0 | 1027 |
| 11:37:30 | 300 | 0.7 | 2.0 | 14.0 | txn BVN1 age 946 | 0 | 117 | 0 | 0 | 1086 |
| 11:38:30 | 300 | 0.8 | 4.2 | 13.4 | txn Directory age 1096 | 0 | 105 | 0 | 0 | 1145 |
| 11:41:43 | 300 | 1.8 | 8007.7 | 8008.2 | chain Directory age 1150 | 16 | 107 | 0 | 16 | 1199 |
| 11:41:44 | 300 | 1.1 | 3.7 | 28.6 | txn BVN2 age 1284 | 0 | 116 | 0 | 0 | 1303 |
| 11:42:45 | 300 | 1.1 | 3.7 | 22.5 | chain BVN1 age 1185 | 0 | 96 | 0 | 0 | 1359 |
