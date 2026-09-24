# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 7152 timed reads, p50 1.2 ms, p95 4.5 ms, p99 15.1 ms, **max 8009.5 ms** (txn read, BVN2, entry 408 blocks old); 906 failed, 40 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 40 | 1.3 | 4.0 | 5.8 |
| 100–1000 | 2926 | 1.4 | 5.0 | 8009.5 |
| 1000–5000 | 4186 | 1.1 | 4.1 | 8008.2 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 3576 | 0.7 | 2.0 | 8008.5 |
| txn | 3576 | 1.9 | 5.9 | 8009.5 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 8009.5 | txn | BVN2 | 408 |
| 8008.5 | chain | BVN3 | 860 |
| 8008.5 | txn | BVN2 | 335 |
| 8008.2 | chain | BVN1 | 1559 |
| 8008.2 | chain | BVN1 | 846 |
| 8008.2 | chain | BVN3 | 860 |
| 8008.2 | txn | BVN1 | 1559 |
| 8008.1 | chain | BVN1 | 1199 |
| 8008.1 | txn | BVN3 | 438 |
| 8008.1 | txn | Directory | 341 |

## Followers (#4365) — does a node in no committee answer?

**acc-bvn3-fol1** (partitions Directory, BVN3): 1799 reads of entries it holds, **1799 answered**, 0 refused (query gate or NotReady), 0 failed; p50 0.4 ms, p95 0.7 ms, max 12.1 ms.

Read from its own port, never through the router: a read that another node could have answered says nothing about this one. Entries of partitions it does not run are not asked for.

**acc-bvn3-fol2**: declared (late-follower profile) and not running at any probe round — not a follower of this run; not probed.


## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 07:48:35 | 8 | 2.9 | 5.8 | 5.8 | txn BVN3 age 35 | 0 | 0 | 0 | 37 |
| 07:49:35 | 32 | 1.3 | 1.7 | 1.7 | txn BVN1 age 95 | 0 | 0 | 0 | 99 |
| 07:50:35 | 56 | 2.0 | 9.2 | 13.4 | txn BVN3 age 154 | 0 | 0 | 0 | 158 |
| 07:51:35 | 80 | 1.6 | 5.2 | 17.0 | txn BVN3 age 214 | 0 | 0 | 0 | 216 |
| 07:52:36 | 104 | 1.4 | 1.9 | 2.3 | txn BVN2 age 273 | 0 | 0 | 0 | 276 |
| 07:55:22 | 128 | 1.6 | 8007.6 | 8008.5 | txn BVN2 age 335 | 12 | 0 | 12 | 341 |
| 07:56:09 | 136 | 1.6 | 11.7 | 8009.5 | txn BVN2 age 408 | 5 | 0 | 5 | 438 |
| 07:56:23 | 144 | 1.3 | 3.4 | 21.3 | txn Directory age 478 | 0 | 0 | 0 | 497 |
| 07:57:23 | 168 | 1.2 | 1.9 | 10.9 | txn Directory age 534 | 0 | 0 | 0 | 557 |
| 07:58:24 | 192 | 1.3 | 2.5 | 13.5 | txn BVN2 age 571 | 0 | 0 | 0 | 616 |
| 07:59:24 | 216 | 1.3 | 2.2 | 5.9 | txn BVN3 age 674 | 0 | 0 | 0 | 674 |
| 08:00:25 | 240 | 1.3 | 3.5 | 25.6 | txn BVN3 age 734 | 0 | 0 | 0 | 734 |
| 08:01:25 | 264 | 1.3 | 2.1 | 19.7 | txn BVN1 age 793 | 0 | 0 | 0 | 793 |
| 08:03:28 | 288 | 1.7 | 12.6 | 8008.5 | chain BVN3 age 860 | 6 | 0 | 6 | 860 |
| 08:03:30 | 296 | 1.8 | 4.9 | 13.9 | txn BVN3 age 916 | 0 | 0 | 0 | 916 |
| 08:04:30 | 300 | 1.5 | 5.7 | 23.9 | txn BVN2 age 931 | 0 | 0 | 0 | 975 |
| 08:05:31 | 300 | 2.1 | 8.1 | 42.9 | txn Directory age 1003 | 0 | 0 | 0 | 1035 |
| 08:06:32 | 300 | 1.3 | 3.0 | 26.7 | txn Directory age 1059 | 24 | 0 | 0 | 1095 |
| 08:07:32 | 300 | 1.1 | 2.1 | 8.6 | txn BVN3 age 1155 | 25 | 0 | 0 | 1155 |
| 08:09:38 | 300 | 1.6 | 6.1 | 8008.1 | chain BVN1 age 1199 | 27 | 0 | 7 | 1217 |
| 08:09:40 | 300 | 1.2 | 2.7 | 16.9 | txn BVN2 age 1233 | 27 | 0 | 0 | 1257 |
| 08:10:40 | 300 | 1.5 | 2.8 | 22.9 | chain BVN3 age 1317 | 22 | 0 | 0 | 1317 |
| 08:11:41 | 300 | 0.9 | 2.9 | 10.1 | txn Directory age 1352 | 47 | 0 | 0 | 1376 |
| 08:12:41 | 300 | 1.2 | 5.7 | 15.1 | txn Directory age 1405 | 75 | 0 | 0 | 1436 |
| 08:13:42 | 300 | 1.0 | 3.7 | 30.0 | chain BVN3 age 1496 | 83 | 0 | 0 | 1496 |
| 08:16:19 | 300 | 1.3 | 27.7 | 8008.2 | chain BVN1 age 1559 | 84 | 0 | 10 | 1561 |
| 08:16:21 | 300 | 1.2 | 4.5 | 17.0 | txn BVN2 age 1597 | 76 | 0 | 0 | 1652 |
| 08:17:21 | 300 | 0.8 | 2.0 | 5.8 | chain BVN2 age 1597 | 82 | 0 | 0 | 1712 |
| 08:18:22 | 300 | 1.2 | 4.6 | 24.3 | txn BVN1 age 1767 | 83 | 0 | 0 | 1771 |
| 08:19:22 | 300 | 0.9 | 2.3 | 12.8 | txn BVN1 age 1827 | 103 | 0 | 0 | 1830 |
| 08:20:23 | 300 | 0.8 | 2.5 | 19.3 | txn BVN2 age 1833 | 125 | 0 | 0 | 1889 |
