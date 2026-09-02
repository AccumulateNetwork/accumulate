# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 1427 timed reads, p50 1.4 ms, p95 30.8 ms, p99 249.1 ms, **max 588.0 ms** (txn read, BVN2, entry 126 blocks old); 0 failed, 0 timed out (8s), 1 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 204 | 1.5 | 61.1 | 348.8 |
| 100–1000 | 1223 | 1.4 | 11.4 | 588.0 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 714 | 0.8 | 2.0 | 348.8 |
| txn | 713 | 2.0 | 101.2 | 588.0 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 588.0 | txn | BVN2 | 126 |
| 409.7 | txn | BVN1 | 170 |
| 383.4 | txn | BVN1 | 132 |
| 348.8 | chain | BVN2 | 71 |
| 317.1 | txn | BVN1 | 68 |
| 286.2 | txn | BVN2 | 71 |
| 286.1 | txn | BVN1 | 251 |
| 285.7 | txn | Directory | 174 |
| 283.9 | txn | Directory | 194 |
| 280.3 | txn | BVN2 | 71 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 05:50:01 | 6 | 1.7 | 1.8 | 1.8 | txn BVN2 age 6 | 0 | 0 | 0 | 8 |
| 05:51:02 | 24 | 1.0 | 1.8 | 1.9 | txn Directory age 33 | 0 | 0 | 0 | 33 |
| 05:52:02 | 42 | 2.1 | 5.4 | 11.0 | chain BVN1 age 50 | 0 | 0 | 0 | 53 |
| 05:53:05 | 60 | 1.6 | 286.2 | 348.8 | chain BVN2 age 71 | 0 | 0 | 0 | 73 |
| 05:54:04 | 72 | 1.8 | 47.1 | 106.2 | txn Directory age 93 | 0 | 0 | 0 | 93 |
| 05:55:04 | 90 | 1.8 | 3.0 | 9.5 | txn BVN2 age 111 | 0 | 0 | 0 | 113 |
| 05:56:07 | 108 | 1.7 | 147.5 | 588.0 | txn BVN2 age 126 | 0 | 1 | 0 | 133 |
| 05:57:06 | 126 | 1.8 | 89.9 | 260.1 | txn BVN2 age 136 | 0 | 0 | 0 | 154 |
| 05:58:08 | 144 | 2.6 | 176.0 | 409.7 | txn BVN1 age 170 | 0 | 0 | 0 | 174 |
| 05:59:07 | 162 | 1.1 | 2.2 | 283.9 | txn Directory age 194 | 0 | 0 | 0 | 194 |
| 06:00:08 | 180 | 1.8 | 21.6 | 262.8 | txn Directory age 213 | 0 | 0 | 0 | 213 |
| 06:01:07 | 198 | 1.2 | 1.8 | 3.9 | txn BVN1 age 231 | 0 | 0 | 0 | 231 |
| 06:02:08 | 216 | 1.5 | 4.2 | 286.1 | txn BVN1 age 251 | 0 | 0 | 0 | 251 |
