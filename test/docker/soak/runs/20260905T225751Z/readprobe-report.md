# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 210 timed reads, p50 1.0 ms, p95 2.1 ms, p99 2.4 ms, **max 2.7 ms** (txn read, BVN2, entry 274 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 90 | 0.9 | 1.9 | 2.4 |
| 100–1000 | 120 | 1.1 | 2.3 | 2.7 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 105 | 0.6 | 1.6 | 2.4 |
| txn | 105 | 1.3 | 2.3 | 2.7 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 2.7 | txn | BVN2 | 274 |
| 2.4 | txn | BVN2 | 274 |
| 2.4 | txn | Directory | 38 |
| 2.4 | chain | BVN2 | 210 |
| 2.3 | txn | BVN2 | 210 |
| 2.3 | txn | BVN1 | 34 |
| 2.3 | txn | Directory | 54 |
| 2.3 | txn | BVN2 | 210 |
| 2.3 | txn | BVN1 | 210 |
| 2.1 | txn | BVN1 | 274 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 23:01:35 | 6 | 1.5 | 2.4 | 2.4 | txn Directory age 38 | 0 | 0 | 0 | 38 |
| 23:02:35 | 24 | 1.2 | 1.9 | 2.0 | txn BVN2 age 94 | 0 | 0 | 0 | 94 |
| 23:03:36 | 42 | 1.0 | 1.4 | 1.6 | txn BVN2 age 154 | 0 | 0 | 0 | 154 |
| 23:04:36 | 60 | 1.2 | 2.3 | 2.4 | chain BVN2 age 210 | 0 | 0 | 0 | 210 |
| 23:05:37 | 78 | 0.9 | 2.1 | 2.7 | txn BVN2 age 274 | 0 | 0 | 0 | 274 |
