# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 3737 timed reads, p50 2.2 ms, p95 61.2 ms, p99 184.8 ms, **max 461.3 ms** (txn read, BVN1, entry 1223 blocks old); 0 failed, 0 timed out (8s), 1 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 30 | 1.1 | 2.2 | 2.2 |
| 100–1000 | 2898 | 1.9 | 30.8 | 383.1 |
| 1000–5000 | 809 | 8.1 | 141.8 | 461.3 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 1869 | 1.0 | 33.9 | 348.6 |
| txn | 1868 | 3.0 | 86.8 | 461.3 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 461.3 | txn | BVN1 | 1223 |
| 383.1 | txn | BVN2 | 810 |
| 348.6 | chain | BVN1 | 1161 |
| 327.8 | txn | BVN1 | 1223 |
| 325.0 | txn | BVN1 | 1161 |
| 298.4 | txn | BVN1 | 1099 |
| 295.0 | txn | BVN1 | 1223 |
| 291.5 | txn | Directory | 1232 |
| 271.9 | chain | BVN1 | 1223 |
| 268.6 | txn | Directory | 1173 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 21:34:20 | 6 | 1.6 | 1.7 | 1.7 | txn Directory age 10 | 0 | 0 | 0 | 10 |
| 21:35:21 | 24 | 1.1 | 2.2 | 2.2 | txn Directory age 97 | 0 | 0 | 0 | 97 |
| 21:36:21 | 42 | 1.8 | 9.5 | 12.4 | txn BVN1 age 152 | 0 | 0 | 0 | 156 |
| 21:37:22 | 60 | 1.2 | 2.0 | 2.1 | txn BVN1 age 212 | 0 | 0 | 0 | 216 |
| 21:38:22 | 78 | 1.8 | 4.9 | 11.8 | txn Directory age 276 | 0 | 0 | 0 | 276 |
| 21:39:22 | 96 | 1.3 | 2.1 | 3.7 | txn BVN2 age 331 | 0 | 0 | 0 | 336 |
| 21:40:23 | 114 | 1.9 | 4.8 | 11.7 | txn Directory age 396 | 0 | 0 | 0 | 396 |
| 21:41:23 | 132 | 1.5 | 2.4 | 3.0 | txn BVN1 age 450 | 0 | 0 | 0 | 456 |
| 21:42:24 | 150 | 1.7 | 13.2 | 31.3 | txn Directory age 515 | 0 | 0 | 0 | 515 |
| 21:43:25 | 168 | 1.8 | 5.0 | 15.8 | txn Directory age 576 | 0 | 0 | 0 | 576 |
| 21:44:26 | 186 | 1.8 | 4.5 | 8.0 | txn BVN2 age 624 | 0 | 0 | 0 | 636 |
| 21:45:26 | 204 | 1.5 | 2.4 | 10.1 | txn Directory age 696 | 0 | 0 | 0 | 696 |
| 21:46:27 | 222 | 2.1 | 6.8 | 31.9 | txn BVN1 age 749 | 0 | 0 | 0 | 756 |
| 21:47:28 | 240 | 2.0 | 6.5 | 13.2 | txn BVN2 age 763 | 0 | 0 | 0 | 816 |
| 21:48:29 | 258 | 2.2 | 6.9 | 38.6 | txn Directory age 876 | 0 | 0 | 0 | 876 |
| 21:49:30 | 270 | 2.2 | 23.2 | 64.0 | txn BVN2 age 809 | 0 | 0 | 0 | 936 |
| 21:50:31 | 288 | 2.5 | 38.9 | 119.2 | chain BVN1 age 982 | 0 | 0 | 0 | 994 |
| 21:51:33 | 300 | 4.5 | 38.8 | 80.9 | txn BVN1 age 1044 | 0 | 0 | 0 | 1056 |
| 21:52:35 | 300 | 5.7 | 80.9 | 298.4 | txn BVN1 age 1099 | 0 | 1 | 0 | 1115 |
| 21:53:46 | 300 | 22.2 | 211.0 | 383.1 | txn BVN2 age 810 | 0 | 0 | 0 | 1173 |
| 21:54:45 | 300 | 18.0 | 202.2 | 461.3 | txn BVN1 age 1223 | 0 | 0 | 0 | 1232 |
