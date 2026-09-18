# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 406 timed reads, p50 1.5 ms, p95 4.3 ms, p99 8040.0 ms, **max 8040.5 ms** (txn read, BVN3, entry 340 blocks old); 16 failed, 11 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 40 | 1.3 | 2.4 | 2.7 |
| 100–1000 | 366 | 1.5 | 4.5 | 8040.5 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 203 | 0.8 | 1.9 | 5.1 |
| txn | 203 | 2.0 | 8012.2 | 8040.5 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 8040.5 | txn | BVN3 | 340 |
| 8040.2 | txn | BVN3 | 340 |
| 8040.1 | txn | BVN1 | 337 |
| 8040.1 | txn | BVN2 | 326 |
| 8040.0 | txn | BVN3 | 340 |
| 8039.8 | txn | BVN2 | 326 |
| 8039.6 | txn | BVN3 | 340 |
| 8039.6 | txn | BVN3 | 340 |
| 8039.6 | txn | Directory | 341 |
| 8039.3 | txn | BVN2 | 326 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 02:19:38 | 8 | 2.1 | 2.7 | 2.7 | txn BVN3 age 34 | 0 | 0 | 0 | 38 |
| 02:20:38 | 32 | 1.3 | 2.1 | 2.2 | txn BVN3 age 94 | 0 | 0 | 0 | 98 |
| 02:21:39 | 56 | 1.1 | 2.6 | 5.0 | chain Directory age 157 | 5 | 0 | 0 | 157 |
| 02:22:39 | 80 | 1.3 | 2.7 | 3.3 | txn Directory age 216 | 0 | 0 | 0 | 216 |
| 02:23:40 | 104 | 1.5 | 3.0 | 5.1 | chain BVN3 age 272 | 0 | 0 | 0 | 276 |
| 02:26:17 | 126 | 1.8 | 8039.6 | 8040.5 | txn BVN3 age 340 | 11 | 0 | 11 | 341 |
