# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 210 timed reads, p50 0.9 ms, p95 1.8 ms, p99 2.1 ms, **max 2.4 ms** (txn read, BVN1, entry 269 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 90 | 0.9 | 1.8 | 2.0 |
| 100–1000 | 120 | 0.9 | 1.7 | 2.4 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 105 | 0.5 | 1.1 | 2.1 |
| txn | 105 | 1.2 | 1.9 | 2.4 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 2.4 | txn | BVN1 | 269 |
| 2.2 | txn | BVN1 | 214 |
| 2.1 | chain | BVN1 | 269 |
| 2.0 | txn | BVN2 | 34 |
| 1.9 | txn | BVN2 | 274 |
| 1.9 | txn | Directory | 54 |
| 1.9 | txn | Directory | 54 |
| 1.9 | txn | BVN1 | 269 |
| 1.8 | txn | BVN1 | 94 |
| 1.8 | txn | Directory | 54 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 23:48:14 | 6 | 1.4 | 2.0 | 2.0 | txn BVN2 age 34 | 0 | 0 | 0 | 38 |
| 23:49:14 | 24 | 0.9 | 1.9 | 1.9 | txn Directory age 54 | 0 | 0 | 0 | 94 |
| 23:50:15 | 42 | 0.8 | 1.6 | 1.7 | txn BVN2 age 154 | 0 | 0 | 0 | 154 |
| 23:51:15 | 60 | 0.8 | 1.6 | 2.2 | txn BVN1 age 214 | 0 | 0 | 0 | 214 |
| 23:52:15 | 78 | 1.0 | 1.9 | 2.4 | txn BVN1 age 269 | 0 | 0 | 0 | 274 |
