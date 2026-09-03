# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 3664 timed reads, p50 2.0 ms, p95 146.1 ms, p99 706.6 ms, **max 2378.5 ms** (txn read, BVN1, entry 232 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 204 | 1.4 | 15.2 | 205.6 |
| 100–1000 | 3460 | 2.0 | 154.2 | 2378.5 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 1832 | 0.8 | 8.6 | 1571.4 |
| txn | 1832 | 2.7 | 276.0 | 2378.5 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 2378.5 | txn | BVN1 | 232 |
| 2346.3 | txn | BVN2 | 294 |
| 2234.1 | txn | Directory | 235 |
| 1749.8 | txn | BVN2 | 262 |
| 1571.4 | chain | BVN1 | 411 |
| 1438.5 | txn | BVN1 | 314 |
| 1435.0 | chain | BVN1 | 353 |
| 1339.5 | txn | BVN2 | 242 |
| 1338.1 | chain | BVN2 | 284 |
| 1281.9 | txn | BVN1 | 272 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 23:19:02 | 6 | 1.5 | 1.7 | 1.7 | txn BVN2 age 13 | 0 | 0 | 0 | 14 |
| 23:20:02 | 24 | 1.0 | 1.5 | 1.8 | txn BVN2 age 33 | 0 | 0 | 0 | 34 |
| 23:21:03 | 42 | 1.5 | 3.0 | 4.6 | txn BVN1 age 50 | 0 | 0 | 0 | 54 |
| 23:22:04 | 60 | 1.9 | 80.4 | 205.6 | txn BVN1 age 71 | 0 | 0 | 0 | 75 |
| 23:23:04 | 72 | 2.2 | 17.8 | 205.0 | txn BVN1 age 90 | 0 | 0 | 0 | 94 |
| 23:24:06 | 90 | 2.1 | 176.1 | 500.9 | txn BVN2 age 113 | 0 | 0 | 0 | 114 |
| 23:25:05 | 108 | 2.0 | 4.3 | 446.2 | txn BVN1 age 132 | 0 | 0 | 0 | 134 |
| 23:26:06 | 126 | 1.8 | 6.3 | 309.8 | txn BVN1 age 153 | 0 | 0 | 0 | 155 |
| 23:27:05 | 144 | 1.7 | 2.9 | 5.6 | txn Directory age 175 | 0 | 0 | 0 | 175 |
| 23:28:13 | 162 | 3.1 | 255.2 | 1246.9 | txn Directory age 195 | 0 | 0 | 0 | 195 |
| 23:29:09 | 180 | 2.5 | 119.4 | 420.1 | txn Directory age 215 | 0 | 0 | 0 | 215 |
| 23:30:24 | 198 | 2.2 | 547.5 | 2378.5 | txn BVN1 age 232 | 0 | 0 | 0 | 235 |
| 23:31:13 | 216 | 2.8 | 145.1 | 1339.5 | txn BVN2 age 242 | 0 | 0 | 0 | 255 |
| 23:32:21 | 234 | 2.8 | 280.4 | 1749.8 | txn BVN2 age 262 | 0 | 0 | 0 | 275 |
| 23:33:14 | 252 | 2.1 | 63.0 | 1338.1 | chain BVN2 age 284 | 0 | 0 | 0 | 295 |
| 23:34:34 | 270 | 3.3 | 626.7 | 2346.3 | txn BVN2 age 294 | 0 | 0 | 0 | 316 |
| 23:35:13 | 282 | 2.0 | 73.0 | 687.9 | txn BVN1 age 333 | 0 | 0 | 0 | 336 |
| 23:36:15 | 298 | 1.9 | 52.0 | 1435.0 | chain BVN1 age 353 | 0 | 0 | 0 | 356 |
| 23:37:19 | 300 | 1.7 | 183.1 | 1136.3 | txn BVN1 age 372 | 0 | 0 | 0 | 376 |
| 23:38:14 | 300 | 1.6 | 79.0 | 609.8 | txn BVN2 age 329 | 0 | 0 | 0 | 396 |
| 23:39:14 | 300 | 2.0 | 4.4 | 1571.4 | chain BVN1 age 411 | 0 | 0 | 0 | 416 |
