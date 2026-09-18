# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 704 timed reads, p50 1.6 ms, p95 3.6 ms, p99 2017.7 ms, **max 8040.1 ms** (txn read, BVN2, entry 335 blocks old); 7 failed, 7 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 40 | 1.5 | 3.4 | 5.3 |
| 100–1000 | 664 | 1.6 | 3.6 | 8040.1 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 352 | 0.7 | 1.6 | 6.6 |
| txn | 352 | 2.1 | 7.4 | 8040.1 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 8040.1 | txn | BVN2 | 335 |
| 8040.0 | txn | Directory | 343 |
| 8039.9 | txn | Directory | 343 |
| 8039.7 | txn | Directory | 343 |
| 8039.5 | txn | BVN1 | 336 |
| 8039.4 | txn | BVN3 | 339 |
| 8027.6 | txn | Directory | 343 |
| 2017.7 | txn | BVN2 | 335 |
| 33.2 | txn | Directory | 217 |
| 22.1 | txn | BVN1 | 336 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 12:47:25 | 8 | 2.0 | 2.4 | 2.4 | txn BVN3 age 35 | 0 | 0 | 0 | 37 |
| 12:48:26 | 32 | 1.5 | 3.4 | 5.3 | txn BVN1 age 94 | 0 | 0 | 0 | 98 |
| 12:49:26 | 56 | 1.3 | 2.7 | 3.3 | txn BVN3 age 153 | 0 | 0 | 0 | 158 |
| 12:50:27 | 80 | 1.7 | 19.3 | 33.2 | txn Directory age 217 | 0 | 0 | 0 | 217 |
| 12:51:27 | 104 | 1.6 | 3.2 | 6.6 | chain BVN1 age 268 | 0 | 0 | 0 | 277 |
| 12:53:34 | 128 | 1.7 | 8027.6 | 8040.1 | txn BVN2 age 335 | 7 | 0 | 7 | 343 |
| 12:53:35 | 136 | 1.6 | 3.1 | 8.8 | txn BVN1 age 395 | 0 | 0 | 0 | 398 |
| 12:54:36 | 160 | 1.4 | 2.6 | 7.0 | txn Directory age 455 | 0 | 0 | 0 | 457 |
