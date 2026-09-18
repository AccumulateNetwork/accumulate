# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 3396 timed reads, p50 2.2 ms, p95 19.0 ms, p99 41.8 ms, **max 121.0 ms** (txn read, BVN2, entry 448 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 30 | 1.4 | 3.7 | 3.9 |
| 100–1000 | 2564 | 2.1 | 22.2 | 121.0 |
| 1000–5000 | 802 | 2.9 | 12.0 | 57.2 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 1698 | 1.0 | 5.9 | 57.0 |
| txn | 1698 | 3.5 | 31.4 | 121.0 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 121.0 | txn | BVN2 | 448 |
| 100.2 | txn | BVN1 | 628 |
| 76.9 | txn | BVN2 | 628 |
| 64.0 | txn | Directory | 636 |
| 63.2 | txn | Directory | 816 |
| 60.2 | txn | Directory | 816 |
| 59.7 | txn | BVN2 | 808 |
| 59.5 | txn | Directory | 517 |
| 58.2 | txn | BVN2 | 448 |
| 58.0 | txn | Directory | 457 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 16:38:15 | 6 | 1.4 | 1.7 | 1.7 | txn BVN1 age 7 | 0 | 0 | 0 | 11 |
| 16:39:16 | 24 | 1.9 | 3.7 | 3.9 | txn Directory age 98 | 0 | 0 | 0 | 98 |
| 16:40:16 | 42 | 1.8 | 8.6 | 17.5 | txn Directory age 158 | 0 | 0 | 0 | 158 |
| 16:41:17 | 60 | 1.7 | 2.2 | 6.7 | txn BVN1 age 212 | 0 | 0 | 0 | 217 |
| 16:42:17 | 78 | 1.4 | 3.8 | 9.6 | txn BVN2 age 276 | 0 | 0 | 0 | 277 |
| 16:43:18 | 96 | 2.3 | 11.4 | 29.0 | txn Directory age 337 | 0 | 0 | 0 | 337 |
| 16:44:18 | 114 | 1.6 | 4.9 | 32.2 | txn BVN1 age 390 | 0 | 0 | 0 | 397 |
| 16:45:19 | 132 | 2.4 | 24.1 | 121.0 | txn BVN2 age 448 | 0 | 0 | 0 | 457 |
| 16:46:19 | 144 | 2.2 | 23.9 | 59.5 | txn Directory age 517 | 0 | 0 | 0 | 517 |
| 16:47:19 | 162 | 2.5 | 11.3 | 40.5 | txn BVN1 age 569 | 0 | 0 | 0 | 576 |
| 16:48:21 | 180 | 2.4 | 37.7 | 100.2 | txn BVN1 age 628 | 0 | 0 | 0 | 636 |
| 16:49:21 | 198 | 2.3 | 23.4 | 44.4 | txn BVN1 age 688 | 0 | 0 | 0 | 696 |
| 16:50:22 | 216 | 2.3 | 31.9 | 50.2 | txn Directory age 756 | 0 | 0 | 0 | 756 |
| 16:51:23 | 234 | 2.7 | 35.2 | 63.2 | txn Directory age 816 | 0 | 0 | 0 | 816 |
| 16:52:23 | 252 | 2.1 | 18.6 | 39.2 | txn BVN1 age 865 | 0 | 0 | 0 | 877 |
| 16:53:24 | 270 | 2.5 | 23.9 | 41.4 | txn Directory age 936 | 0 | 0 | 0 | 936 |
| 16:54:25 | 288 | 3.1 | 22.5 | 55.6 | txn Directory age 996 | 0 | 0 | 0 | 996 |
| 16:55:25 | 300 | 3.6 | 13.0 | 57.8 | txn BVN2 age 983 | 0 | 0 | 0 | 1056 |
| 16:56:26 | 300 | 3.0 | 10.6 | 51.4 | txn Directory age 1114 | 0 | 0 | 0 | 1114 |
| 16:57:26 | 300 | 2.5 | 13.0 | 49.7 | txn BVN2 age 1045 | 0 | 0 | 0 | 1175 |
