# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 4950 timed reads, p50 1.1 ms, p95 4.0 ms, p99 11.7 ms, **max 317.7 ms** (txn read, BVN1, entry 1279 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 30 | 1.3 | 8.6 | 14.4 |
| 100–1000 | 2520 | 1.1 | 2.5 | 15.0 |
| 1000–5000 | 2400 | 1.2 | 6.4 | 317.7 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 2475 | 0.7 | 1.9 | 37.4 |
| txn | 2475 | 1.6 | 5.8 | 317.7 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 317.7 | txn | BVN1 | 1279 |
| 163.6 | txn | BVN2 | 1277 |
| 130.0 | txn | BVN1 | 1279 |
| 81.0 | txn | BVN2 | 1277 |
| 68.4 | txn | Directory | 1294 |
| 56.4 | txn | BVN1 | 1279 |
| 47.3 | txn | BVN2 | 1277 |
| 43.7 | txn | BVN1 | 1396 |
| 40.6 | txn | Directory | 1354 |
| 37.4 | txn | BVN1 | 1344 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 18:31:49 | 6 | 1.7 | 1.9 | 1.9 | txn Directory age 10 | 0 | 0 | 0 | 10 |
| 18:32:50 | 24 | 1.3 | 8.6 | 14.4 | chain BVN2 age 92 | 0 | 0 | 0 | 96 |
| 18:33:50 | 42 | 1.1 | 1.9 | 2.4 | txn BVN1 age 152 | 0 | 0 | 0 | 156 |
| 18:34:50 | 60 | 1.0 | 2.2 | 3.5 | txn BVN1 age 212 | 0 | 0 | 0 | 216 |
| 18:35:51 | 78 | 1.2 | 2.7 | 3.8 | txn BVN2 age 271 | 0 | 0 | 0 | 275 |
| 18:36:51 | 96 | 1.2 | 3.5 | 7.9 | txn BVN2 age 330 | 0 | 0 | 0 | 335 |
| 18:37:52 | 114 | 1.0 | 2.1 | 4.2 | txn BVN1 age 390 | 0 | 0 | 0 | 395 |
| 18:38:52 | 132 | 1.2 | 3.1 | 13.7 | chain BVN2 age 449 | 0 | 0 | 0 | 455 |
| 18:39:53 | 150 | 1.2 | 2.1 | 14.9 | txn Directory age 514 | 0 | 0 | 0 | 514 |
| 18:40:53 | 168 | 1.2 | 3.9 | 15.0 | txn Directory age 574 | 0 | 0 | 0 | 574 |
| 18:41:54 | 186 | 1.4 | 4.0 | 8.4 | txn BVN2 age 628 | 0 | 0 | 0 | 634 |
| 18:42:54 | 204 | 1.1 | 2.1 | 6.3 | txn BVN2 age 688 | 0 | 0 | 0 | 694 |
| 18:43:55 | 222 | 1.0 | 1.4 | 2.7 | txn BVN1 age 748 | 0 | 0 | 0 | 754 |
| 18:44:56 | 240 | 1.2 | 2.6 | 9.2 | txn BVN2 age 807 | 0 | 0 | 0 | 814 |
| 18:45:56 | 258 | 1.1 | 2.2 | 5.4 | chain BVN1 age 867 | 0 | 0 | 0 | 874 |
| 18:46:57 | 276 | 1.0 | 1.6 | 3.7 | txn Directory age 934 | 0 | 0 | 0 | 934 |
| 18:47:58 | 294 | 1.1 | 3.0 | 6.5 | txn BVN1 age 987 | 0 | 0 | 0 | 994 |
| 18:48:58 | 300 | 0.9 | 1.8 | 4.9 | txn Directory age 1054 | 0 | 0 | 0 | 1054 |
| 18:49:59 | 300 | 1.4 | 4.4 | 18.2 | chain BVN1 age 1106 | 0 | 0 | 0 | 1114 |
| 18:51:00 | 300 | 1.2 | 2.8 | 19.0 | txn Directory age 1174 | 0 | 0 | 0 | 1174 |
| 18:52:01 | 300 | 1.3 | 5.3 | 15.6 | txn BVN1 age 1226 | 0 | 0 | 0 | 1234 |
| 18:53:03 | 300 | 1.6 | 13.0 | 317.7 | txn BVN1 age 1279 | 0 | 0 | 0 | 1294 |
| 18:54:02 | 300 | 1.7 | 11.5 | 40.6 | txn Directory age 1354 | 0 | 0 | 0 | 1354 |
| 18:55:02 | 300 | 1.7 | 8.7 | 43.7 | txn BVN1 age 1396 | 0 | 0 | 0 | 1413 |
| 18:56:02 | 300 | 1.2 | 2.7 | 5.7 | txn Directory age 1473 | 0 | 0 | 0 | 1473 |
