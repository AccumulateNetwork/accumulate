# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 8612 timed reads, p50 1.2 ms, p95 3.0 ms, p99 25.1 ms, **max 117.7 ms** (txn read, BVN2, entry 1337 blocks old); 0 failed (0 of them timed out, 8s), 564 refused NotReady (a joining node's designed answer; not timed), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 40 | 1.4 | 4.1 | 4.4 |
| 100–1000 | 3094 | 1.1 | 2.2 | 64.2 |
| 1000–5000 | 5478 | 1.2 | 4.1 | 117.7 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 4486 | 0.6 | 1.3 | 82.3 |
| txn | 4126 | 1.5 | 5.5 | 117.7 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 117.7 | txn | BVN2 | 1337 |
| 115.7 | txn | Directory | 1414 |
| 114.6 | txn | BVN3 | 1583 |
| 92.9 | txn | BVN1 | 1523 |
| 82.3 | chain | BVN1 | 1642 |
| 79.0 | txn | Directory | 1115 |
| 78.7 | txn | BVN1 | 1403 |
| 77.0 | txn | BVN1 | 1523 |
| 70.2 | txn | Directory | 1115 |
| 70.0 | txn | BVN1 | 1583 |

## Followers (#4365) — does a node in no committee answer?

**acc-bvn1-fol1** (partitions Directory, BVN1): 1183 reads of entries it holds, **714 answered**, 392 refused (query gate or NotReady), 77 failed; p50 0.3 ms, p95 54.1 ms, max 168.8 ms.

Read from its own port, never through the router: a read that another node could have answered says nothing about this one. Entries of partitions it does not run are not asked for.

**acc-bvn2-fol1** (partitions Directory, BVN2): 1233 reads of entries it holds, **609 answered**, 549 refused (query gate or NotReady), 75 failed; p50 0.3 ms, p95 47.6 ms, max 120.8 ms.

Read from its own port, never through the router: a read that another node could have answered says nothing about this one. Entries of partitions it does not run are not asked for.

**acc-bvn3-fol1** (partitions Directory, BVN3): 2280 reads of entries it holds, **2280 answered**, 0 refused (query gate or NotReady), 0 failed; p50 0.3 ms, p95 0.5 ms, max 7.9 ms.

Read from its own port, never through the router: a read that another node could have answered says nothing about this one. Entries of partitions it does not run are not asked for.

**acc-bvn3-fol2** (partitions Directory, BVN3): 1196 reads of entries it holds, **859 answered**, 267 refused (query gate or NotReady), 70 failed; p50 0.3 ms, p95 48.7 ms, max 146.4 ms.

Read from its own port, never through the router: a read that another node could have answered says nothing about this one. Entries of partitions it does not run are not asked for.


## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | NotReady | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|---|
| 15:14:04 | 8 | 3.4 | 4.4 | 4.4 | txn BVN1 age 36 | 0 | 0 | 0 | 0 | 40 |
| 15:15:04 | 32 | 1.3 | 1.7 | 1.7 | txn BVN2 age 95 | 0 | 0 | 0 | 0 | 99 |
| 15:16:04 | 56 | 1.0 | 1.4 | 1.7 | txn BVN3 age 155 | 0 | 0 | 0 | 0 | 159 |
| 15:17:04 | 80 | 1.1 | 1.4 | 1.6 | txn Directory age 218 | 0 | 0 | 0 | 0 | 218 |
| 15:18:04 | 104 | 1.1 | 2.0 | 9.4 | txn BVN2 age 272 | 0 | 0 | 0 | 0 | 278 |
| 15:19:04 | 128 | 1.3 | 2.6 | 5.3 | txn BVN3 age 333 | 0 | 0 | 0 | 0 | 338 |
| 15:20:05 | 152 | 1.0 | 1.4 | 1.6 | txn BVN3 age 392 | 0 | 0 | 0 | 0 | 397 |
| 15:21:05 | 176 | 1.6 | 5.7 | 29.0 | txn BVN3 age 451 | 0 | 0 | 0 | 0 | 457 |
| 15:22:05 | 200 | 1.1 | 1.8 | 24.6 | txn BVN3 age 511 | 0 | 0 | 0 | 0 | 517 |
| 15:23:06 | 224 | 1.1 | 1.4 | 2.0 | txn Directory age 577 | 0 | 0 | 0 | 0 | 577 |
| 15:24:06 | 248 | 1.2 | 3.3 | 22.8 | txn BVN3 age 630 | 0 | 0 | 0 | 0 | 636 |
| 15:25:06 | 272 | 1.2 | 1.9 | 24.5 | txn Directory age 696 | 0 | 0 | 0 | 0 | 696 |
| 15:26:07 | 296 | 1.2 | 2.4 | 26.6 | txn BVN2 age 744 | 0 | 0 | 0 | 0 | 756 |
| 15:27:07 | 300 | 1.3 | 3.2 | 64.2 | txn BVN1 age 808 | 0 | 0 | 0 | 0 | 816 |
| 15:28:08 | 300 | 1.1 | 1.5 | 2.1 | txn BVN2 age 863 | 0 | 0 | 0 | 0 | 876 |
| 15:29:08 | 300 | 1.2 | 1.8 | 27.4 | txn BVN3 age 928 | 0 | 0 | 0 | 0 | 936 |
| 15:30:09 | 300 | 0.7 | 1.8 | 5.0 | txn BVN3 age 987 | 0 | 42 | 0 | 0 | 996 |
| 15:31:11 | 300 | 1.3 | 12.0 | 53.9 | txn Directory age 1056 | 0 | 47 | 0 | 0 | 1056 |
| 15:32:11 | 300 | 1.1 | 10.3 | 79.0 | txn Directory age 1115 | 0 | 44 | 0 | 0 | 1115 |
| 15:33:12 | 300 | 1.0 | 11.1 | 68.8 | chain BVN3 age 1165 | 0 | 50 | 0 | 0 | 1175 |
| 15:34:12 | 300 | 0.9 | 5.9 | 52.1 | chain BVN2 age 1219 | 0 | 53 | 0 | 0 | 1235 |
| 15:35:12 | 300 | 1.2 | 4.7 | 25.9 | txn BVN1 age 1284 | 0 | 43 | 0 | 0 | 1295 |
| 15:36:13 | 300 | 1.3 | 7.2 | 67.9 | chain Directory age 1354 | 0 | 48 | 0 | 0 | 1354 |
| 15:37:14 | 300 | 1.5 | 23.0 | 117.7 | txn BVN2 age 1337 | 0 | 41 | 0 | 0 | 1414 |
| 15:38:14 | 300 | 1.4 | 12.6 | 67.0 | txn Directory age 1475 | 0 | 33 | 0 | 0 | 1475 |
| 15:39:14 | 300 | 1.2 | 6.4 | 92.9 | txn BVN1 age 1523 | 0 | 36 | 0 | 0 | 1534 |
| 15:40:15 | 300 | 1.5 | 4.9 | 114.6 | txn BVN3 age 1583 | 0 | 32 | 0 | 0 | 1594 |
| 15:41:15 | 300 | 1.3 | 4.4 | 82.3 | chain BVN1 age 1642 | 0 | 30 | 0 | 0 | 1654 |
| 15:42:15 | 300 | 0.8 | 2.1 | 23.4 | chain BVN3 age 1702 | 0 | 33 | 0 | 0 | 1714 |
| 15:43:15 | 300 | 1.3 | 2.7 | 33.7 | txn BVN2 age 1753 | 0 | 32 | 0 | 0 | 1774 |
| 15:44:15 | 300 | 1.3 | 2.8 | 42.7 | txn BVN3 age 1821 | 0 | 0 | 0 | 0 | 1834 |
| 15:45:16 | 300 | 1.1 | 1.6 | 13.0 | txn BVN1 age 1880 | 0 | 0 | 0 | 0 | 1894 |
| 15:46:16 | 300 | 1.2 | 1.8 | 22.9 | txn Directory age 1954 | 0 | 0 | 0 | 0 | 1954 |
| 15:47:16 | 300 | 1.2 | 1.6 | 37.7 | txn BVN2 age 1990 | 0 | 0 | 0 | 0 | 2013 |
| 15:48:17 | 300 | 1.3 | 3.8 | 29.9 | txn BVN3 age 2060 | 0 | 0 | 0 | 0 | 2073 |
| 15:49:17 | 300 | 1.2 | 2.0 | 31.6 | txn BVN1 age 2118 | 0 | 0 | 0 | 0 | 2133 |
| 15:50:18 | 300 | 1.2 | 2.0 | 33.4 | txn BVN2 age 2168 | 0 | 0 | 0 | 0 | 2193 |
