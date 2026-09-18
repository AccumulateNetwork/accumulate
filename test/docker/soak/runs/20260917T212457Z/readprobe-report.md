# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 4076 timed reads, p50 1.4 ms, p95 2.9 ms, p99 6.0 ms, **max 21.0 ms** (chain read, BVN2, entry 332 blocks old); 9 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 40 | 1.7 | 3.3 | 3.9 |
| 100–1000 | 3136 | 1.4 | 2.8 | 21.0 |
| 1000–5000 | 900 | 1.4 | 3.0 | 17.9 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 2038 | 0.7 | 1.6 | 21.0 |
| txn | 2038 | 1.9 | 3.3 | 15.6 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 21.0 | chain | BVN2 | 332 |
| 17.9 | chain | Directory | 1029 |
| 16.2 | chain | BVN3 | 745 |
| 15.6 | txn | BVN2 | 332 |
| 15.5 | txn | BVN2 | 987 |
| 14.4 | txn | BVN3 | 153 |
| 14.1 | txn | BVN2 | 1107 |
| 13.1 | chain | BVN2 | 153 |
| 12.6 | chain | BVN2 | 570 |
| 11.4 | txn | Directory | 858 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 21:26:38 | 8 | 2.5 | 3.9 | 3.9 | txn BVN3 age 34 | 0 | 0 | 0 | 36 |
| 21:27:38 | 32 | 1.6 | 2.6 | 3.3 | txn BVN2 age 94 | 0 | 0 | 0 | 98 |
| 21:28:39 | 56 | 1.7 | 9.6 | 14.4 | txn BVN3 age 153 | 0 | 0 | 0 | 157 |
| 21:29:39 | 80 | 1.3 | 2.3 | 2.7 | txn BVN3 age 212 | 0 | 0 | 0 | 217 |
| 21:30:40 | 104 | 1.2 | 2.5 | 3.7 | txn BVN1 age 271 | 9 | 0 | 0 | 276 |
| 21:31:40 | 128 | 1.4 | 2.9 | 21.0 | chain BVN2 age 332 | 0 | 0 | 0 | 336 |
| 21:32:41 | 152 | 1.4 | 2.4 | 8.8 | txn BVN2 age 391 | 0 | 0 | 0 | 396 |
| 21:33:41 | 176 | 1.4 | 2.9 | 10.4 | txn BVN2 age 451 | 0 | 0 | 0 | 456 |
| 21:34:42 | 200 | 1.5 | 2.9 | 8.1 | txn BVN3 age 506 | 0 | 0 | 0 | 516 |
| 21:35:43 | 224 | 1.3 | 2.6 | 12.6 | chain BVN2 age 570 | 0 | 0 | 0 | 576 |
| 21:36:43 | 248 | 1.4 | 3.0 | 6.2 | chain BVN1 age 626 | 0 | 0 | 0 | 636 |
| 21:37:44 | 272 | 1.3 | 2.3 | 3.2 | txn BVN2 age 689 | 0 | 0 | 0 | 696 |
| 21:38:45 | 296 | 1.4 | 3.2 | 16.2 | chain BVN3 age 745 | 0 | 0 | 0 | 748 |
| 21:39:46 | 300 | 1.5 | 3.0 | 11.2 | txn BVN2 age 808 | 0 | 0 | 0 | 808 |
| 21:40:46 | 300 | 1.5 | 3.1 | 11.4 | txn Directory age 858 | 0 | 0 | 0 | 868 |
| 21:41:47 | 300 | 1.5 | 2.6 | 8.9 | txn BVN3 age 925 | 0 | 0 | 0 | 928 |
| 21:42:48 | 300 | 1.5 | 3.3 | 15.5 | txn BVN2 age 987 | 0 | 0 | 0 | 987 |
| 21:43:49 | 300 | 1.4 | 3.1 | 17.9 | chain Directory age 1029 | 0 | 0 | 0 | 1047 |
| 21:44:49 | 300 | 1.4 | 3.0 | 14.1 | txn BVN2 age 1107 | 0 | 0 | 0 | 1107 |
| 21:45:50 | 300 | 1.4 | 2.9 | 10.7 | txn BVN1 age 1162 | 0 | 0 | 0 | 1167 |
