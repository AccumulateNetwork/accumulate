# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 3599 timed reads, p50 1.9 ms, p95 60.7 ms, p99 397.2 ms, **max 8040.5 ms** (chain read, BVN2, entry 253 blocks old); 8 failed, 8 timed out (8s), 1 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 210 | 1.5 | 35.5 | 231.9 |
| 100–1000 | 3389 | 1.9 | 61.3 | 8040.5 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 1800 | 0.9 | 4.7 | 8040.5 |
| txn | 1799 | 2.9 | 121.8 | 2114.2 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 8040.5 | chain | BVN2 | 253 |
| 8040.4 | chain | BVN1 | 233 |
| 8040.2 | chain | BVN1 | 233 |
| 8040.2 | chain | BVN2 | 253 |
| 8040.1 | chain | Directory | 255 |
| 8039.9 | chain | BVN2 | 253 |
| 8039.7 | chain | BVN1 | 233 |
| 8021.2 | chain | BVN2 | 253 |
| 8020.9 | chain | BVN1 | 233 |
| 2114.2 | txn | Directory | 236 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 04:13:33 | 6 | 1.6 | 2.0 | 2.0 | txn BVN1 age 7 | 0 | 0 | 0 | 8 |
| 04:14:34 | 24 | 1.1 | 65.1 | 170.8 | txn BVN1 age 32 | 0 | 0 | 0 | 34 |
| 04:15:35 | 42 | 1.8 | 19.8 | 231.9 | txn BVN2 age 53 | 0 | 0 | 0 | 55 |
| 04:16:36 | 60 | 1.7 | 10.0 | 51.0 | chain BVN2 age 73 | 0 | 0 | 0 | 75 |
| 04:17:37 | 78 | 1.6 | 64.9 | 139.3 | txn BVN1 age 89 | 0 | 0 | 0 | 95 |
| 04:18:40 | 96 | 1.9 | 96.4 | 970.6 | txn BVN1 age 110 | 0 | 0 | 0 | 115 |
| 04:19:39 | 114 | 1.9 | 9.0 | 352.1 | txn BVN2 age 133 | 0 | 0 | 0 | 135 |
| 04:20:40 | 126 | 2.0 | 5.6 | 375.7 | txn BVN1 age 150 | 0 | 0 | 0 | 156 |
| 04:21:42 | 144 | 2.3 | 102.6 | 319.9 | txn Directory age 176 | 0 | 0 | 0 | 176 |
| 04:22:42 | 162 | 2.5 | 73.9 | 349.2 | txn Directory age 196 | 0 | 0 | 0 | 196 |
| 04:23:46 | 180 | 2.7 | 281.9 | 915.5 | txn BVN2 age 212 | 0 | 0 | 0 | 216 |
| 04:24:46 | 198 | 3.0 | 81.5 | 2114.2 | txn Directory age 236 | 0 | 0 | 0 | 236 |
| 04:27:07 | 214 | 2.3 | 501.0 | 8040.5 | chain BVN2 age 253 | 8 | 0 | 8 | 255 |
| 04:27:08 | 220 | 2.0 | 3.5 | 77.0 | txn BVN2 age 282 | 0 | 0 | 0 | 282 |
| 04:28:15 | 232 | 4.1 | 147.5 | 1394.3 | txn BVN1 age 291 | 0 | 0 | 0 | 300 |
| 04:29:13 | 250 | 3.4 | 88.0 | 599.2 | txn BVN1 age 310 | 0 | 0 | 0 | 320 |
| 04:30:13 | 268 | 1.8 | 48.0 | 590.0 | chain BVN2 age 320 | 0 | 1 | 0 | 341 |
| 04:31:11 | 286 | 1.7 | 3.4 | 98.3 | txn BVN1 age 352 | 0 | 0 | 0 | 361 |
| 04:32:15 | 300 | 1.8 | 12.4 | 840.3 | txn BVN2 age 320 | 0 | 0 | 0 | 380 |
| 04:33:15 | 300 | 1.7 | 38.8 | 892.1 | txn Directory age 385 | 0 | 0 | 0 | 390 |
| 04:34:14 | 300 | 1.7 | 26.9 | 326.5 | txn Directory age 387 | 0 | 0 | 0 | 411 |
