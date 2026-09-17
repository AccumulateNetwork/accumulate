# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 6 timed reads, p50 1.5 ms, p95 2.5 ms, p99 2.5 ms, **max 2.5 ms** (txn read, BVN1, entry 33 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 6 | 1.5 | 2.5 | 2.5 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 3 | 1.0 | 1.1 | 1.1 |
| txn | 3 | 2.0 | 2.5 | 2.5 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 2.5 | txn | BVN1 | 33 |
| 2.0 | txn | BVN2 | 33 |
| 1.5 | txn | Directory | 36 |
| 1.1 | chain | BVN1 | 33 |
| 1.0 | chain | BVN2 | 33 |
| 0.8 | chain | Directory | 36 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 00:21:35 | 6 | 1.5 | 2.5 | 2.5 | txn BVN1 age 33 | 0 | 0 | 0 | 36 |
