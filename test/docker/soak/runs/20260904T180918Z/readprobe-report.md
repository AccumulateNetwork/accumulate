# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 3444 timed reads, p50 1.8 ms, p95 14.0 ms, p99 41.1 ms, **max 116.1 ms** (chain read, Directory, entry 754 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 30 | 1.3 | 1.9 | 4.3 |
| 100–1000 | 2614 | 1.8 | 10.9 | 116.1 |
| 1000–5000 | 800 | 2.4 | 25.6 | 102.2 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 1722 | 0.9 | 4.2 | 116.1 |
| txn | 1722 | 2.6 | 29.0 | 102.2 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 116.1 | chain | Directory | 754 |
| 102.2 | txn | BVN1 | 1069 |
| 90.6 | txn | BVN2 | 861 |
| 88.5 | txn | BVN2 | 861 |
| 85.9 | txn | Directory | 754 |
| 82.4 | txn | BVN1 | 845 |
| 70.4 | txn | BVN2 | 861 |
| 70.1 | txn | Directory | 874 |
| 58.7 | chain | Directory | 754 |
| 58.7 | txn | Directory | 934 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 18:12:17 | 6 | 1.6 | 1.7 | 1.7 | txn Directory age 10 | 0 | 0 | 0 | 10 |
| 18:13:17 | 24 | 1.3 | 1.9 | 4.3 | txn BVN1 age 96 | 0 | 0 | 0 | 97 |
| 18:14:18 | 42 | 1.0 | 1.7 | 2.5 | txn BVN2 age 153 | 0 | 0 | 0 | 157 |
| 18:15:18 | 60 | 1.7 | 2.4 | 9.3 | txn Directory age 217 | 0 | 0 | 0 | 217 |
| 18:16:18 | 78 | 1.2 | 3.5 | 17.5 | chain BVN2 age 272 | 0 | 0 | 0 | 276 |
| 18:17:19 | 96 | 1.5 | 2.8 | 4.1 | chain BVN1 age 333 | 0 | 0 | 0 | 336 |
| 18:18:19 | 114 | 1.3 | 4.1 | 12.0 | txn BVN2 age 390 | 0 | 0 | 0 | 396 |
| 18:19:20 | 132 | 1.4 | 2.3 | 11.8 | txn BVN2 age 450 | 0 | 0 | 0 | 456 |
| 18:20:20 | 150 | 1.6 | 3.5 | 10.0 | chain Directory age 515 | 0 | 0 | 0 | 515 |
| 18:21:21 | 168 | 1.8 | 4.9 | 11.6 | txn BVN1 age 569 | 0 | 0 | 0 | 575 |
| 18:22:22 | 186 | 2.0 | 5.3 | 17.0 | txn BVN1 age 631 | 0 | 0 | 0 | 635 |
| 18:23:22 | 204 | 1.6 | 3.5 | 13.5 | chain BVN1 age 688 | 0 | 0 | 0 | 695 |
| 18:24:24 | 222 | 2.5 | 36.7 | 116.1 | chain Directory age 754 | 0 | 0 | 0 | 754 |
| 18:25:24 | 240 | 1.9 | 10.7 | 35.6 | txn BVN1 age 800 | 0 | 0 | 0 | 814 |
| 18:26:26 | 258 | 2.5 | 35.2 | 90.6 | txn BVN2 age 861 | 0 | 0 | 0 | 874 |
| 18:27:26 | 276 | 2.2 | 10.7 | 58.7 | txn Directory age 934 | 0 | 0 | 0 | 934 |
| 18:28:26 | 288 | 2.4 | 29.1 | 46.8 | txn Directory age 994 | 0 | 0 | 0 | 994 |
| 18:29:27 | 300 | 3.0 | 26.1 | 47.7 | txn BVN1 age 984 | 0 | 0 | 0 | 1053 |
| 18:30:28 | 300 | 2.5 | 28.1 | 54.9 | txn BVN1 age 1022 | 0 | 0 | 0 | 1113 |
| 18:31:28 | 300 | 2.2 | 23.6 | 102.2 | txn BVN1 age 1069 | 0 | 0 | 0 | 1173 |
