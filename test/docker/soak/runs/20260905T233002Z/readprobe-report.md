# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 210 timed reads, p50 1.1 ms, p95 2.7 ms, p99 4.3 ms, **max 4.9 ms** (txn read, BVN2, entry 154 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 90 | 1.1 | 2.6 | 4.3 |
| 100–1000 | 120 | 1.1 | 2.7 | 4.9 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 105 | 0.7 | 1.4 | 2.0 |
| txn | 105 | 1.4 | 3.1 | 4.9 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 4.9 | txn | BVN2 | 154 |
| 4.3 | txn | BVN2 | 154 |
| 4.3 | txn | Directory | 54 |
| 3.3 | txn | Directory | 54 |
| 3.2 | txn | BVN1 | 93 |
| 3.1 | txn | BVN2 | 94 |
| 3.1 | txn | BVN1 | 274 |
| 3.0 | txn | BVN1 | 274 |
| 2.7 | txn | BVN2 | 210 |
| 2.7 | txn | BVN1 | 274 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 23:33:37 | 6 | 1.8 | 1.9 | 1.9 | txn BVN1 age 37 | 0 | 0 | 0 | 38 |
| 23:34:37 | 24 | 1.3 | 3.1 | 3.2 | txn BVN1 age 93 | 0 | 0 | 0 | 94 |
| 23:35:38 | 42 | 1.1 | 4.3 | 4.9 | txn BVN2 age 154 | 0 | 0 | 0 | 154 |
| 23:36:38 | 60 | 1.0 | 2.4 | 2.7 | txn BVN2 age 210 | 0 | 0 | 0 | 210 |
| 23:37:39 | 78 | 1.3 | 2.7 | 3.3 | txn Directory age 54 | 0 | 0 | 0 | 274 |
