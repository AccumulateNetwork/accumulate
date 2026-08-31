# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 4650 timed reads, p50 1.5 ms, p95 5.3 ms, p99 37.0 ms, **max 8040.5 ms** (chain read, BVN1, entry 434 blocks old); 2 failed, 2 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 210 | 1.1 | 1.9 | 3.6 |
| 100–1000 | 4440 | 1.5 | 5.5 | 8040.5 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 2325 | 0.8 | 2.8 | 8040.5 |
| txn | 2325 | 2.1 | 8.4 | 1267.7 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 8040.5 | chain | BVN1 | 434 |
| 8039.8 | chain | Directory | 422 |
| 6818.8 | chain | Directory | 422 |
| 1267.7 | txn | BVN2 | 368 |
| 193.6 | txn | BVN2 | 368 |
| 116.5 | txn | BVN1 | 413 |
| 109.2 | txn | BVN1 | 374 |
| 107.8 | chain | BVN1 | 393 |
| 107.7 | txn | BVN1 | 353 |
| 103.7 | txn | Directory | 416 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 06:02:08 | 6 | 1.7 | 1.9 | 1.9 | txn BVN1 age 6 | 0 | 0 | 0 | 8 |
| 06:03:08 | 24 | 1.0 | 1.5 | 3.1 | txn Directory age 34 | 0 | 0 | 0 | 34 |
| 06:04:08 | 42 | 1.4 | 2.4 | 3.6 | txn Directory age 54 | 0 | 0 | 0 | 54 |
| 06:05:09 | 60 | 1.4 | 2.0 | 2.6 | txn Directory age 74 | 0 | 0 | 0 | 74 |
| 06:06:09 | 78 | 1.3 | 1.8 | 3.5 | chain BVN2 age 92 | 0 | 0 | 0 | 94 |
| 06:07:10 | 96 | 1.4 | 2.1 | 5.7 | txn BVN1 age 112 | 0 | 0 | 0 | 114 |
| 06:08:10 | 114 | 1.5 | 3.5 | 44.1 | txn BVN1 age 133 | 0 | 0 | 0 | 134 |
| 06:09:11 | 132 | 2.1 | 9.4 | 87.0 | txn BVN2 age 152 | 0 | 0 | 0 | 154 |
| 06:10:12 | 150 | 1.2 | 1.8 | 49.9 | txn BVN2 age 173 | 0 | 0 | 0 | 174 |
| 06:11:12 | 168 | 1.0 | 1.6 | 3.0 | txn BVN1 age 193 | 0 | 0 | 0 | 195 |
| 06:12:13 | 186 | 2.4 | 6.7 | 11.2 | txn BVN1 age 212 | 0 | 0 | 0 | 215 |
| 06:13:14 | 204 | 1.5 | 2.4 | 10.5 | chain BVN2 age 232 | 0 | 0 | 0 | 235 |
| 06:14:15 | 222 | 1.9 | 3.1 | 6.4 | txn Directory age 255 | 0 | 0 | 0 | 255 |
| 06:15:15 | 240 | 2.1 | 3.8 | 76.5 | txn BVN2 age 268 | 0 | 0 | 0 | 275 |
| 06:16:17 | 258 | 2.0 | 19.5 | 46.3 | chain Directory age 295 | 0 | 0 | 0 | 295 |
| 06:17:17 | 276 | 1.5 | 2.4 | 5.7 | txn Directory age 316 | 0 | 0 | 0 | 316 |
| 06:18:17 | 294 | 1.3 | 2.1 | 5.0 | txn BVN2 age 310 | 0 | 0 | 0 | 336 |
| 06:19:19 | 300 | 1.6 | 4.9 | 107.7 | txn BVN1 age 353 | 0 | 0 | 0 | 356 |
| 06:20:19 | 300 | 1.6 | 5.2 | 109.2 | txn BVN1 age 374 | 0 | 0 | 0 | 376 |
| 06:21:19 | 300 | 1.8 | 7.5 | 107.8 | chain BVN1 age 393 | 0 | 0 | 0 | 396 |
| 06:22:21 | 300 | 2.4 | 28.8 | 193.6 | txn BVN2 age 368 | 0 | 0 | 0 | 416 |
| 06:23:45 | 300 | 1.5 | 23.3 | 8040.5 | chain BVN1 age 434 | 2 | 0 | 2 | 434 |
| 06:24:21 | 300 | 1.2 | 2.1 | 4.6 | txn BVN2 age 368 | 0 | 0 | 0 | 454 |
| 06:25:21 | 300 | 1.7 | 3.5 | 5.2 | txn BVN2 age 368 | 0 | 0 | 0 | 474 |
