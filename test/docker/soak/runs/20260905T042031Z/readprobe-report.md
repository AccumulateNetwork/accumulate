# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 1326 timed reads, p50 0.6 ms, p95 6.7 ms, p99 22.4 ms, **max 59.0 ms** (txn read, Directory, entry 158 blocks old); 624 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 30 | 1.5 | 19.4 | 25.6 |
| 100–1000 | 1296 | 0.6 | 6.5 | 59.0 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 663 | 0.6 | 1.7 | 13.9 |
| txn | 663 | 1.6 | 13.8 | 59.0 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 59.0 | txn | Directory | 158 |
| 41.0 | txn | BVN2 | 451 |
| 39.8 | txn | Directory | 458 |
| 39.7 | txn | BVN2 | 153 |
| 38.6 | txn | Directory | 458 |
| 29.5 | txn | Directory | 458 |
| 28.8 | txn | BVN1 | 153 |
| 27.5 | txn | BVN1 | 451 |
| 25.6 | txn | BVN2 | 94 |
| 24.1 | txn | BVN1 | 212 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 04:23:47 | 6 | 1.5 | 1.7 | 1.7 | txn BVN2 age 16 | 0 | 0 | 0 | 16 |
| 04:24:47 | 24 | 3.2 | 19.4 | 25.6 | txn BVN2 age 94 | 0 | 0 | 0 | 98 |
| 04:25:48 | 42 | 2.2 | 28.8 | 59.0 | txn Directory age 158 | 0 | 0 | 0 | 158 |
| 04:26:48 | 60 | 2.0 | 21.9 | 24.1 | txn BVN1 age 212 | 0 | 0 | 0 | 218 |
| 04:27:49 | 78 | 1.3 | 15.2 | 21.3 | txn BVN2 age 272 | 0 | 0 | 0 | 278 |
| 04:28:49 | 96 | 1.7 | 12.9 | 22.9 | txn BVN1 age 331 | 0 | 0 | 0 | 338 |
| 04:29:50 | 114 | 2.0 | 10.0 | 17.4 | txn BVN1 age 391 | 0 | 0 | 0 | 398 |
| 04:30:51 | 132 | 1.9 | 20.5 | 41.0 | txn BVN2 age 451 | 0 | 0 | 0 | 458 |
| 04:31:52 | 150 | 1.7 | 4.2 | 8.0 | txn BVN1 age 510 | 0 | 0 | 0 | 518 |
| 04:32:52 | 156 | 0.0 | 0.0 | 0.1 | chain Directory age 518 | 156 | 0 | 0 | 518 |
| 04:33:52 | 156 | 0.0 | 0.1 | 0.1 | chain Directory age 518 | 156 | 0 | 0 | 518 |
| 04:34:53 | 156 | 0.0 | 0.0 | 0.1 | chain BVN2 age 511 | 156 | 0 | 0 | 518 |
| 04:35:53 | 156 | 0.0 | 0.1 | 0.1 | txn BVN2 age 511 | 156 | 0 | 0 | 518 |
