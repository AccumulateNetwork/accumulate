# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 3144 timed reads, p50 1.4 ms, p95 15.1 ms, p99 50.1 ms, **max 198.2 ms** (txn read, BVN1, entry 1097 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 30 | 1.1 | 4.2 | 5.1 |
| 100–1000 | 2514 | 1.4 | 11.0 | 95.5 |
| 1000–5000 | 600 | 2.0 | 34.3 | 198.2 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 1572 | 0.8 | 4.1 | 66.8 |
| txn | 1572 | 1.8 | 32.6 | 198.2 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 198.2 | txn | BVN1 | 1097 |
| 127.6 | txn | Directory | 1114 |
| 115.4 | txn | Directory | 1114 |
| 98.7 | txn | Directory | 1114 |
| 95.5 | txn | BVN1 | 918 |
| 83.4 | txn | BVN1 | 1097 |
| 82.9 | txn | BVN1 | 1097 |
| 74.1 | txn | Directory | 1114 |
| 71.6 | txn | BVN2 | 1105 |
| 67.5 | txn | BVN1 | 746 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 20:28:07 | 6 | 2.9 | 5.1 | 5.1 | txn Directory age 10 | 0 | 0 | 0 | 10 |
| 20:29:07 | 24 | 1.1 | 1.5 | 1.6 | txn BVN1 age 92 | 0 | 0 | 0 | 96 |
| 20:30:07 | 42 | 1.3 | 4.2 | 21.9 | txn BVN2 age 152 | 0 | 0 | 0 | 156 |
| 20:31:08 | 60 | 1.4 | 39.1 | 49.0 | txn BVN2 age 211 | 0 | 0 | 0 | 216 |
| 20:32:09 | 78 | 1.5 | 4.0 | 42.5 | txn BVN2 age 271 | 0 | 0 | 0 | 276 |
| 20:33:09 | 96 | 1.1 | 11.0 | 39.1 | txn Directory age 336 | 0 | 0 | 0 | 336 |
| 20:34:10 | 114 | 1.4 | 4.9 | 22.0 | txn BVN2 age 390 | 0 | 0 | 0 | 395 |
| 20:35:10 | 132 | 1.4 | 6.8 | 33.1 | txn BVN1 age 448 | 0 | 0 | 0 | 455 |
| 20:36:11 | 150 | 1.3 | 3.8 | 14.0 | chain BVN1 age 508 | 0 | 0 | 0 | 515 |
| 20:37:11 | 168 | 1.2 | 2.1 | 7.8 | chain BVN1 age 567 | 0 | 0 | 0 | 575 |
| 20:38:13 | 186 | 1.5 | 35.8 | 62.9 | txn BVN1 age 627 | 0 | 0 | 0 | 636 |
| 20:39:13 | 204 | 1.5 | 4.3 | 66.1 | txn BVN1 age 687 | 0 | 0 | 0 | 695 |
| 20:40:15 | 222 | 1.5 | 31.3 | 67.5 | txn BVN1 age 746 | 0 | 0 | 0 | 756 |
| 20:41:15 | 240 | 1.2 | 23.3 | 42.3 | txn Directory age 815 | 0 | 0 | 0 | 815 |
| 20:42:15 | 258 | 1.2 | 6.0 | 65.1 | txn BVN1 age 861 | 0 | 0 | 0 | 874 |
| 20:43:16 | 276 | 1.4 | 10.6 | 95.5 | txn BVN1 age 918 | 0 | 0 | 0 | 935 |
| 20:44:17 | 288 | 2.4 | 11.4 | 64.7 | txn BVN2 age 975 | 0 | 0 | 0 | 995 |
| 20:45:17 | 300 | 1.8 | 9.8 | 49.5 | txn BVN2 age 1043 | 0 | 0 | 0 | 1055 |
| 20:46:20 | 300 | 2.4 | 57.8 | 198.2 | txn BVN1 age 1097 | 0 | 0 | 0 | 1114 |
