# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 176 timed reads, p50 1.2 ms, p95 2.0 ms, p99 3.4 ms, **max 3.5 ms** (txn read, BVN2, entry 213 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 156 | 1.2 | 2.0 | 3.4 |
| 100–1000 | 20 | 1.4 | 3.5 | 3.5 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 88 | 0.6 | 1.0 | 2.2 |
| txn | 88 | 1.5 | 2.4 | 3.5 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 3.5 | txn | BVN2 | 213 |
| 3.4 | txn | BVN1 | 1 |
| 3.0 | txn | BVN3 | 1 |
| 2.4 | txn | Directory | 1 |
| 2.4 | txn | BVN1 | 1 |
| 2.3 | txn | BVN3 | 1 |
| 2.2 | chain | BVN1 | 1 |
| 2.1 | txn | BVN2 | 1 |
| 2.0 | txn | Directory | 1 |
| 2.0 | txn | Directory | 1 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 13:19:08 | 8 | 2.2 | 3.4 | 3.4 | txn BVN1 age 1 | 0 | 0 | 0 | 1 |
| 13:20:08 | 32 | 1.3 | 1.5 | 1.6 | txn BVN3 age 1 | 0 | 0 | 0 | 1 |
| 13:21:09 | 56 | 1.2 | 2.0 | 2.4 | txn BVN1 age 1 | 0 | 0 | 0 | 1 |
| 13:22:09 | 80 | 1.2 | 1.7 | 3.5 | txn BVN2 age 213 | 0 | 0 | 0 | 213 |
