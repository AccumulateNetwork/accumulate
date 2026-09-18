# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 552 timed reads, p50 1.9 ms, p95 6.3 ms, p99 17.4 ms, **max 57.3 ms** (txn read, BVN2, entry 215 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 22 | 1.1 | 2.1 | 2.8 |
| 100–1000 | 530 | 2.0 | 6.7 | 57.3 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 276 | 1.0 | 3.7 | 17.2 |
| txn | 276 | 2.5 | 8.1 | 57.3 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 57.3 | txn | BVN2 | 215 |
| 45.6 | txn | BVN1 | 155 |
| 29.0 | txn | Directory | 160 |
| 27.9 | txn | BVN1 | 333 |
| 18.9 | txn | BVN2 | 215 |
| 17.4 | txn | BVN1 | 450 |
| 17.2 | chain | BVN2 | 334 |
| 16.8 | txn | Directory | 339 |
| 16.1 | chain | BVN1 | 214 |
| 16.0 | txn | BVN2 | 334 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 23:58:03 | 6 | 1.6 | 2.0 | 2.0 | txn BVN2 age 7 | 0 | 0 | 0 | 10 |
| 23:59:03 | 24 | 1.1 | 2.1 | 2.8 | txn BVN1 age 96 | 0 | 0 | 0 | 100 |
| 00:00:04 | 42 | 1.7 | 4.9 | 45.6 | txn BVN1 age 155 | 0 | 0 | 0 | 160 |
| 00:01:04 | 60 | 2.2 | 16.1 | 57.3 | txn BVN2 age 215 | 0 | 0 | 0 | 220 |
| 00:02:05 | 78 | 1.9 | 7.5 | 9.7 | chain BVN2 age 274 | 0 | 0 | 0 | 280 |
| 00:03:06 | 96 | 2.1 | 14.2 | 27.9 | txn BVN1 age 333 | 0 | 0 | 0 | 339 |
| 00:04:06 | 114 | 1.9 | 3.9 | 9.7 | txn BVN2 age 393 | 0 | 0 | 0 | 399 |
| 00:05:07 | 132 | 2.3 | 4.9 | 17.4 | txn BVN1 age 450 | 0 | 0 | 0 | 459 |
