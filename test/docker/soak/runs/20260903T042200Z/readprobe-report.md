# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 6 timed reads, p50 1.4 ms, p95 2.4 ms, p99 2.4 ms, **max 2.4 ms** (txn read, BVN1, entry 12 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 6 | 1.4 | 2.4 | 2.4 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 3 | 0.9 | 1.0 | 1.0 |
| txn | 3 | 2.0 | 2.4 | 2.4 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 2.4 | txn | BVN1 | 12 |
| 2.0 | txn | Directory | 12 |
| 1.4 | txn | BVN2 | 6 |
| 1.0 | chain | BVN2 | 6 |
| 0.9 | chain | BVN1 | 12 |
| 0.8 | chain | Directory | 12 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 04:24:46 | 6 | 1.4 | 2.4 | 2.4 | txn BVN1 age 12 | 0 | 0 | 0 | 12 |
