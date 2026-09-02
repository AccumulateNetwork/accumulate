# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 3844 timed reads, p50 1.7 ms, p95 105.8 ms, p99 466.7 ms, **max 8040.6 ms** (chain read, BVN1, entry 192 blocks old); 11 failed, 11 timed out (8s), 2 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 192 | 1.4 | 104.9 | 168.9 |
| 100–1000 | 3652 | 1.8 | 108.7 | 8040.6 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 1922 | 0.9 | 9.8 | 8040.6 |
| txn | 1922 | 2.4 | 182.7 | 7503.3 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 8040.6 | chain | BVN1 | 192 |
| 8040.5 | chain | BVN2 | 216 |
| 8040.2 | chain | Directory | 215 |
| 8040.0 | chain | BVN1 | 192 |
| 8040.0 | chain | BVN1 | 192 |
| 8039.8 | chain | Directory | 215 |
| 8039.8 | chain | BVN2 | 216 |
| 8039.6 | chain | BVN2 | 216 |
| 8039.6 | chain | Directory | 215 |
| 8039.5 | chain | BVN2 | 216 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 13:29:03 | 6 | 1.6 | 1.7 | 1.7 | txn BVN2 age 6 | 0 | 0 | 0 | 8 |
| 13:30:04 | 24 | 1.1 | 78.8 | 168.9 | txn Directory age 34 | 0 | 0 | 0 | 34 |
| 13:31:04 | 36 | 1.4 | 116.4 | 130.5 | txn Directory age 54 | 0 | 0 | 0 | 54 |
| 13:32:05 | 54 | 1.9 | 104.9 | 137.3 | txn BVN1 age 72 | 0 | 0 | 0 | 74 |
| 13:33:05 | 72 | 1.5 | 105.6 | 122.8 | txn BVN2 age 93 | 0 | 0 | 0 | 94 |
| 13:34:06 | 90 | 1.6 | 117.7 | 358.1 | txn Directory age 114 | 0 | 0 | 0 | 114 |
| 13:35:06 | 108 | 1.4 | 20.8 | 282.1 | txn Directory age 135 | 0 | 0 | 0 | 135 |
| 13:36:07 | 126 | 1.8 | 38.5 | 335.9 | txn BVN2 age 149 | 0 | 0 | 0 | 155 |
| 13:37:13 | 144 | 2.0 | 197.9 | 1246.7 | txn Directory age 175 | 0 | 0 | 0 | 175 |
| 13:38:11 | 162 | 1.8 | 236.8 | 481.2 | txn BVN1 age 192 | 0 | 0 | 0 | 195 |
| 13:40:57 | 180 | 1.7 | 8039.6 | 8040.6 | chain BVN1 age 192 | 11 | 0 | 11 | 216 |
| 13:41:01 | 186 | 1.5 | 49.3 | 382.4 | txn Directory age 248 | 0 | 0 | 0 | 250 |
| 13:42:03 | 198 | 1.9 | 189.1 | 677.9 | txn Directory age 268 | 0 | 0 | 0 | 268 |
| 13:43:08 | 216 | 2.2 | 244.8 | 1565.5 | txn BVN2 age 283 | 0 | 0 | 0 | 288 |
| 13:44:03 | 234 | 2.4 | 44.3 | 823.4 | chain BVN2 age 298 | 0 | 2 | 0 | 308 |
| 13:45:12 | 252 | 2.4 | 194.6 | 2107.9 | txn BVN2 age 298 | 0 | 0 | 0 | 321 |
| 13:46:12 | 270 | 2.3 | 262.1 | 892.6 | txn BVN1 age 341 | 0 | 0 | 0 | 341 |
| 13:47:05 | 288 | 2.5 | 36.0 | 939.5 | txn BVN1 age 358 | 0 | 0 | 0 | 369 |
| 13:48:03 | 300 | 1.8 | 6.8 | 325.8 | chain BVN1 age 374 | 0 | 0 | 0 | 389 |
| 13:49:03 | 300 | 1.7 | 3.6 | 179.8 | txn Directory age 409 | 0 | 0 | 0 | 409 |
| 13:50:03 | 300 | 1.6 | 2.6 | 5.3 | txn BVN2 age 347 | 0 | 0 | 0 | 427 |
| 13:51:03 | 300 | 1.2 | 2.4 | 3.2 | txn Directory age 436 | 0 | 0 | 0 | 436 |
