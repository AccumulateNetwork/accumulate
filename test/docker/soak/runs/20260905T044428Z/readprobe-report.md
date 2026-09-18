# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 4038 timed reads, p50 1.9 ms, p95 12.5 ms, p99 42.3 ms, **max 104.1 ms** (txn read, BVN1, entry 1103 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 30 | 1.6 | 10.7 | 12.5 |
| 100–1000 | 2508 | 1.7 | 7.1 | 53.5 |
| 1000–5000 | 1500 | 2.3 | 23.4 | 104.1 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 2019 | 0.9 | 5.0 | 45.3 |
| txn | 2019 | 2.5 | 20.1 | 104.1 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 104.1 | txn | BVN1 | 1103 |
| 103.2 | txn | BVN2 | 1198 |
| 97.3 | txn | Directory | 1232 |
| 95.9 | txn | BVN2 | 1153 |
| 94.7 | txn | BVN2 | 1198 |
| 92.1 | txn | BVN1 | 1221 |
| 87.5 | txn | BVN1 | 1221 |
| 83.7 | txn | Directory | 1232 |
| 83.2 | txn | BVN2 | 1153 |
| 72.1 | txn | BVN2 | 1198 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 04:47:44 | 6 | 1.6 | 1.9 | 1.9 | txn BVN1 age 12 | 0 | 0 | 0 | 15 |
| 04:48:45 | 24 | 1.6 | 10.7 | 12.5 | txn Directory age 96 | 0 | 0 | 0 | 96 |
| 04:49:45 | 42 | 1.9 | 8.0 | 12.0 | txn BVN1 age 152 | 0 | 0 | 0 | 156 |
| 04:50:45 | 60 | 1.8 | 3.4 | 13.2 | txn BVN2 age 211 | 0 | 0 | 0 | 216 |
| 04:51:46 | 78 | 1.7 | 3.7 | 6.9 | txn BVN1 age 271 | 0 | 0 | 0 | 276 |
| 04:52:46 | 96 | 1.8 | 4.0 | 12.3 | txn Directory age 335 | 0 | 0 | 0 | 335 |
| 04:53:47 | 114 | 1.0 | 1.8 | 2.7 | txn Directory age 395 | 0 | 0 | 0 | 395 |
| 04:54:47 | 132 | 1.7 | 6.5 | 12.5 | txn BVN1 age 449 | 0 | 0 | 0 | 455 |
| 04:55:48 | 150 | 1.4 | 2.2 | 5.5 | txn Directory age 515 | 0 | 0 | 0 | 515 |
| 04:56:49 | 168 | 1.7 | 4.2 | 12.9 | chain Directory age 575 | 0 | 0 | 0 | 575 |
| 04:57:49 | 186 | 1.6 | 4.0 | 34.1 | txn BVN1 age 628 | 0 | 0 | 0 | 635 |
| 04:58:50 | 204 | 1.9 | 6.6 | 30.0 | chain BVN2 age 685 | 0 | 0 | 0 | 695 |
| 04:59:51 | 222 | 1.7 | 4.7 | 16.6 | txn BVN2 age 742 | 0 | 0 | 0 | 755 |
| 05:00:52 | 240 | 2.1 | 16.6 | 53.5 | txn Directory age 815 | 0 | 0 | 0 | 815 |
| 05:01:52 | 258 | 2.1 | 6.9 | 35.9 | txn BVN1 age 867 | 0 | 0 | 0 | 874 |
| 05:02:53 | 270 | 2.2 | 13.5 | 39.0 | txn BVN1 age 926 | 0 | 0 | 0 | 934 |
| 05:03:53 | 288 | 2.1 | 8.5 | 46.2 | txn BVN2 age 974 | 0 | 0 | 0 | 993 |
| 05:04:53 | 300 | 2.0 | 8.6 | 58.2 | txn Directory age 1053 | 0 | 0 | 0 | 1053 |
| 05:05:54 | 300 | 2.2 | 13.4 | 104.1 | txn BVN1 age 1103 | 0 | 0 | 0 | 1112 |
| 05:06:55 | 300 | 2.3 | 35.4 | 95.9 | txn BVN2 age 1153 | 0 | 0 | 0 | 1172 |
| 05:07:57 | 300 | 3.3 | 50.5 | 103.2 | txn BVN2 age 1198 | 0 | 0 | 0 | 1232 |
| 05:08:55 | 300 | 2.2 | 11.8 | 66.3 | txn BVN1 age 1280 | 0 | 0 | 0 | 1292 |
