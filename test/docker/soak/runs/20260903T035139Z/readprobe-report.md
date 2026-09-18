# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 6900 timed reads, p50 0.7 ms, p95 4.7 ms, p99 157.7 ms, **max 8040.2 ms** (chain read, Directory, entry 231 blocks old); 2434 failed, 7 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 3790 | 0.0 | 3.6 | 8039.9 |
| 100–1000 | 3110 | 1.0 | 6.0 | 8040.2 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 3450 | 0.6 | 1.5 | 8040.2 |
| txn | 3450 | 1.5 | 9.5 | 828.3 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 8040.2 | chain | Directory | 231 |
| 8039.9 | chain | BVN1 | 43 |
| 8039.7 | chain | BVN2 | 185 |
| 8039.6 | chain | BVN2 | 185 |
| 8039.5 | chain | BVN2 | 185 |
| 8036.1 | chain | BVN1 | 43 |
| 8001.7 | chain | Directory | 231 |
| 2964.5 | chain | BVN1 | 43 |
| 1615.6 | chain | BVN2 | 56 |
| 828.3 | txn | BVN1 | 43 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 03:54:07 | 6 | 1.5 | 1.9 | 1.9 | txn Directory age 14 | 0 | 0 | 0 | 14 |
| 03:55:08 | 24 | 1.0 | 1.7 | 2.1 | txn Directory age 34 | 0 | 0 | 0 | 34 |
| 03:56:08 | 42 | 1.0 | 1.6 | 1.8 | txn Directory age 54 | 0 | 0 | 0 | 54 |
| 03:57:09 | 60 | 1.3 | 52.6 | 145.2 | txn BVN2 age 69 | 0 | 0 | 0 | 75 |
| 03:58:10 | 78 | 1.1 | 54.2 | 103.7 | txn Directory age 95 | 0 | 0 | 0 | 95 |
| 03:59:13 | 96 | 1.5 | 106.2 | 1615.6 | chain BVN2 age 56 | 0 | 0 | 0 | 115 |
| 04:00:10 | 108 | 1.4 | 2.0 | 2.1 | txn BVN2 age 107 | 0 | 0 | 0 | 135 |
| 04:01:11 | 126 | 1.7 | 2.3 | 2.8 | txn BVN2 age 126 | 0 | 0 | 0 | 155 |
| 04:02:12 | 144 | 2.0 | 3.0 | 3.5 | txn Directory age 175 | 0 | 0 | 0 | 175 |
| 04:03:13 | 162 | 2.6 | 7.0 | 162.5 | txn BVN2 age 156 | 0 | 0 | 0 | 195 |
| 04:04:14 | 180 | 1.4 | 46.4 | 667.9 | chain BVN2 age 171 | 0 | 0 | 0 | 215 |
| 04:06:20 | 198 | 1.4 | 126.6 | 8040.2 | chain Directory age 231 | 7 | 0 | 7 | 231 |
| 04:06:22 | 204 | 1.2 | 2.0 | 24.8 | chain Directory age 252 | 0 | 0 | 0 | 252 |
| 04:07:22 | 222 | 1.7 | 2.7 | 5.8 | txn BVN2 age 213 | 0 | 0 | 0 | 274 |
| 04:08:29 | 240 | 2.7 | 198.8 | 544.3 | txn BVN1 age 43 | 0 | 0 | 0 | 294 |
| 04:09:24 | 252 | 1.3 | 3.3 | 198.3 | txn Directory age 314 | 0 | 0 | 0 | 314 |
| 04:10:28 | 270 | 1.8 | 136.7 | 406.3 | txn Directory age 334 | 0 | 0 | 0 | 334 |
| 04:11:26 | 288 | 2.1 | 5.9 | 353.8 | txn Directory age 354 | 0 | 0 | 0 | 354 |
| 04:12:28 | 300 | 1.7 | 50.0 | 564.5 | txn BVN2 age 56 | 0 | 0 | 0 | 374 |
| 04:13:26 | 300 | 2.1 | 9.9 | 423.6 | txn Directory age 394 | 0 | 0 | 0 | 394 |
| 04:14:29 | 300 | 1.6 | 60.3 | 750.7 | chain BVN2 age 316 | 0 | 0 | 0 | 414 |
| 04:15:26 | 300 | 1.8 | 4.7 | 828.3 | txn BVN1 age 43 | 0 | 0 | 0 | 434 |
| 04:16:25 | 300 | 0.0 | 0.0 | 0.1 | chain BVN1 age 43 | 300 | 0 | 0 | 434 |
| 04:17:25 | 300 | 0.0 | 0.0 | 0.1 | txn BVN2 age 331 | 300 | 0 | 0 | 434 |
| 04:18:26 | 300 | 0.7 | 1.3 | 62.1 | txn BVN1 age 43 | 27 | 0 | 0 | 43 |
| 04:19:26 | 300 | 0.0 | 0.0 | 0.1 | chain BVN1 age 43 | 300 | 0 | 0 | 43 |
| 04:20:27 | 300 | 0.0 | 0.0 | 0.1 | chain Directory age 2 | 300 | 0 | 0 | 43 |
| 04:21:27 | 300 | 0.0 | 0.0 | 0.0 | chain BVN2 age 1 | 300 | 0 | 0 | 43 |
| 04:22:27 | 300 | 0.0 | 0.0 | 0.1 | txn BVN1 age 43 | 300 | 0 | 0 | 43 |
| 04:23:28 | 300 | 0.0 | 0.0 | 0.1 | txn BVN1 age 43 | 300 | 0 | 0 | 43 |
| 04:24:29 | 300 | 0.9 | 1.4 | 118.3 | txn BVN2 age 6 | 0 | 0 | 0 | 9 |
| 04:25:29 | 300 | 0.0 | 0.0 | 2.5 | chain BVN2 age 6 | 300 | 0 | 0 | 9 |
