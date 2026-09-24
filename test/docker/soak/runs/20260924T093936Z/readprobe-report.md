# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 5744 timed reads, p50 1.6 ms, p95 7.7 ms, p99 32.7 ms, **max 8015.2 ms** (chain read, BVN3, entry 688 blocks old); 1541 failed, 25 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 40 | 2.9 | 6.8 | 9.4 |
| 100–1000 | 3080 | 1.8 | 10.0 | 8015.2 |
| 1000–5000 | 2624 | 1.4 | 6.1 | 8008.5 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 2872 | 0.9 | 4.6 | 8015.2 |
| txn | 2872 | 2.3 | 10.4 | 8008.5 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 8015.2 | chain | BVN3 | 688 |
| 8009.6 | chain | Directory | 406 |
| 8009.1 | chain | BVN1 | 741 |
| 8008.5 | txn | BVN1 | 1556 |
| 8008.4 | chain | BVN3 | 688 |
| 8008.2 | txn | Directory | 1551 |
| 8008.2 | chain | BVN3 | 688 |
| 8008.2 | chain | BVN1 | 741 |
| 8008.1 | chain | BVN2 | 666 |
| 8008.1 | txn | Directory | 738 |

## Followers (#4365) — does a node in no committee answer?

**acc-bvn3-fol1** (partitions Directory, BVN3): 1429 reads of entries it holds, **1429 answered**, 0 refused (query gate or NotReady), 0 failed; p50 0.5 ms, p95 1.8 ms, max 177.9 ms.

Read from its own port, never through the router: a read that another node could have answered says nothing about this one. Entries of partitions it does not run are not asked for.

**acc-bvn3-fol2**: declared (late-follower profile) and not running at any probe round — not a follower of this run; not probed.


## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 09:41:09 | 8 | 2.4 | 5.9 | 5.9 | txn BVN1 age 35 | 0 | 0 | 0 | 37 |
| 09:42:10 | 32 | 3.0 | 6.8 | 9.4 | txn BVN1 age 95 | 0 | 0 | 0 | 99 |
| 09:43:10 | 56 | 2.4 | 10.0 | 16.0 | txn BVN2 age 155 | 0 | 0 | 0 | 159 |
| 09:44:10 | 80 | 1.3 | 6.5 | 13.5 | txn BVN3 age 214 | 7 | 0 | 0 | 218 |
| 09:45:11 | 104 | 1.0 | 1.6 | 2.5 | txn BVN1 age 270 | 9 | 0 | 0 | 275 |
| 09:46:11 | 128 | 3.0 | 7.2 | 18.5 | txn Directory age 335 | 11 | 0 | 0 | 335 |
| 09:48:11 | 150 | 1.9 | 30.1 | 8009.6 | chain Directory age 406 | 6 | 0 | 5 | 408 |
| 09:48:12 | 158 | 2.5 | 13.7 | 32.4 | txn BVN1 age 448 | 3 | 0 | 0 | 452 |
| 09:49:13 | 182 | 2.1 | 4.3 | 22.0 | chain BVN1 age 508 | 7 | 0 | 0 | 511 |
| 09:50:14 | 206 | 3.5 | 24.2 | 54.1 | chain BVN3 age 571 | 4 | 0 | 0 | 571 |
| 09:51:14 | 228 | 2.5 | 9.4 | 140.6 | txn BVN2 age 607 | 56 | 0 | 0 | 629 |
| 09:52:14 | 252 | 1.2 | 2.1 | 18.5 | chain BVN1 age 686 | 68 | 0 | 0 | 688 |
| 09:55:18 | 276 | 2.4 | 6166.8 | 8015.2 | chain BVN3 age 688 | 79 | 0 | 13 | 741 |
| 09:55:21 | 284 | 3.0 | 15.1 | 167.5 | txn BVN2 age 849 | 65 | 0 | 0 | 871 |
| 09:56:21 | 300 | 2.7 | 16.6 | 53.3 | txn BVN2 age 909 | 66 | 0 | 0 | 930 |
| 09:57:21 | 300 | 1.2 | 2.8 | 22.0 | txn BVN1 age 952 | 71 | 0 | 0 | 968 |
| 09:58:22 | 300 | 1.3 | 7.8 | 23.6 | txn BVN1 age 1011 | 107 | 0 | 0 | 1011 |
| 09:59:23 | 300 | 1.2 | 3.6 | 29.0 | txn BVN1 age 1071 | 111 | 0 | 0 | 1110 |
| 10:00:23 | 300 | 0.9 | 2.8 | 16.0 | txn BVN3 age 1169 | 94 | 0 | 0 | 1169 |
| 10:01:42 | 300 | 1.3 | 4.8 | 8007.5 | txn Directory age 1145 | 81 | 0 | 1 | 1219 |
| 10:02:24 | 300 | 1.7 | 4.8 | 22.6 | txn BVN1 age 1249 | 92 | 0 | 0 | 1264 |
| 10:03:26 | 300 | 2.7 | 14.9 | 78.5 | chain BVN1 age 1309 | 97 | 0 | 0 | 1324 |
| 10:04:26 | 300 | 1.5 | 5.3 | 29.3 | txn BVN3 age 1383 | 131 | 0 | 0 | 1383 |
| 10:05:26 | 300 | 1.3 | 5.1 | 26.6 | txn BVN2 age 1443 | 124 | 0 | 0 | 1443 |
| 10:06:27 | 300 | 1.0 | 3.6 | 11.8 | chain BVN1 age 1488 | 117 | 0 | 0 | 1503 |
| 10:08:25 | 300 | 1.3 | 6.9 | 8008.5 | txn BVN1 age 1556 | 135 | 0 | 6 | 1571 |
