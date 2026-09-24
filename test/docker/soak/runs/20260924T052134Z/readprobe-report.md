# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 4880 timed reads, p50 1.8 ms, p95 8.0 ms, p99 30.6 ms, **max 8013.3 ms** (txn read, BVN2, entry 381 blocks old); 795 failed, 28 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 40 | 1.6 | 17.1 | 24.7 |
| 100–1000 | 3118 | 1.8 | 7.6 | 8013.3 |
| 1000–5000 | 1722 | 1.8 | 8.4 | 8008.2 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 2440 | 0.9 | 4.3 | 8008.2 |
| txn | 2440 | 2.4 | 10.7 | 8013.3 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 8013.3 | txn | BVN2 | 381 |
| 8009.8 | txn | BVN2 | 381 |
| 8009.1 | txn | Directory | 384 |
| 8008.8 | txn | BVN2 | 381 |
| 8008.2 | txn | Directory | 1248 |
| 8008.2 | chain | BVN3 | 771 |
| 8008.2 | txn | BVN1 | 399 |
| 8008.1 | chain | BVN2 | 799 |
| 8008.1 | chain | BVN1 | 822 |
| 8008.1 | txn | BVN3 | 1286 |

## Followers (#4365) — does a node in no committee answer?

**acc-bvn3-fol1** (partitions Directory, BVN3): 1206 reads of entries it holds, **1206 answered**, 0 refused (query gate or NotReady), 0 failed; p50 0.5 ms, p95 1.4 ms, max 7.8 ms.

Read from its own port, never through the router: a read that another node could have answered says nothing about this one. Entries of partitions it does not run are not asked for.

**acc-bvn3-fol2** (partitions Directory, BVN3): 1206 reads of entries it holds, **0 answered**, 0 refused (query gate or NotReady), 1206 failed; p50 0.0 ms, p95 0.1 ms, max 48.7 ms.

Read from its own port, never through the router: a read that another node could have answered says nothing about this one. Entries of partitions it does not run are not asked for.


## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 05:23:02 | 8 | 2.5 | 3.9 | 3.9 | txn BVN2 age 34 | 0 | 0 | 0 | 39 |
| 05:24:02 | 32 | 1.6 | 17.1 | 24.7 | txn Directory age 99 | 0 | 0 | 0 | 99 |
| 05:25:02 | 56 | 1.7 | 4.0 | 5.4 | txn Directory age 159 | 0 | 0 | 0 | 159 |
| 05:26:02 | 80 | 1.6 | 7.5 | 13.8 | txn BVN1 age 213 | 0 | 0 | 0 | 217 |
| 05:27:02 | 104 | 1.6 | 2.2 | 3.2 | txn Directory age 270 | 0 | 0 | 0 | 273 |
| 05:28:03 | 128 | 2.6 | 9.0 | 21.0 | txn BVN3 age 332 | 0 | 0 | 0 | 332 |
| 05:30:27 | 152 | 2.5 | 8007.6 | 8013.3 | txn BVN2 age 381 | 9 | 0 | 9 | 400 |
| 05:30:28 | 160 | 1.8 | 3.8 | 10.3 | txn BVN3 age 476 | 0 | 0 | 0 | 476 |
| 05:31:29 | 184 | 1.7 | 3.0 | 9.4 | txn BVN1 age 214 | 0 | 0 | 0 | 535 |
| 05:32:29 | 208 | 1.7 | 2.6 | 6.1 | txn BVN2 age 553 | 0 | 0 | 0 | 595 |
| 05:33:30 | 232 | 1.9 | 6.4 | 33.4 | chain BVN1 age 214 | 34 | 0 | 0 | 652 |
| 05:34:30 | 256 | 1.4 | 3.5 | 30.7 | chain Directory age 693 | 33 | 0 | 0 | 712 |
| 05:35:31 | 280 | 1.6 | 5.3 | 69.0 | chain BVN1 age 772 | 49 | 0 | 0 | 772 |
| 05:38:20 | 300 | 2.0 | 22.3 | 8008.2 | chain BVN3 age 771 | 59 | 0 | 12 | 822 |
| 05:38:21 | 300 | 1.7 | 5.9 | 20.4 | txn Directory age 770 | 42 | 0 | 0 | 939 |
| 05:39:22 | 300 | 2.2 | 8.2 | 34.0 | txn BVN2 age 959 | 39 | 0 | 0 | 999 |
| 05:40:22 | 300 | 2.3 | 7.0 | 32.4 | chain Directory age 1016 | 43 | 0 | 0 | 1058 |
| 05:41:22 | 300 | 1.6 | 6.7 | 25.0 | txn BVN3 age 1117 | 78 | 0 | 0 | 1117 |
| 05:42:23 | 300 | 1.4 | 3.9 | 24.9 | txn BVN3 age 1177 | 70 | 0 | 0 | 1177 |
| 05:43:25 | 300 | 3.0 | 16.0 | 60.2 | txn BVN2 age 1196 | 89 | 0 | 0 | 1237 |
| 05:45:27 | 300 | 1.8 | 12.5 | 8008.2 | txn Directory age 1248 | 92 | 0 | 7 | 1286 |
| 05:45:29 | 300 | 1.2 | 4.3 | 11.1 | txn BVN3 age 1268 | 81 | 0 | 0 | 1319 |
| 05:46:30 | 300 | 2.4 | 14.7 | 47.6 | txn Directory age 1363 | 77 | 0 | 0 | 1392 |
