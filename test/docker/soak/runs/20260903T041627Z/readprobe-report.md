# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 6 timed reads, p50 1.7 ms, p95 2.2 ms, p99 2.2 ms, **max 2.2 ms** (txn read, BVN1, entry 12 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 6 | 1.7 | 2.2 | 2.2 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 3 | 0.9 | 0.9 | 0.9 |
| txn | 3 | 1.7 | 2.2 | 2.2 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 2.2 | txn | BVN1 | 12 |
| 1.7 | txn | Directory | 13 |
| 1.7 | txn | BVN2 | 12 |
| 0.9 | chain | Directory | 13 |
| 0.9 | chain | BVN1 | 12 |
| 0.9 | chain | BVN2 | 12 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 04:19:00 | 6 | 1.7 | 2.2 | 2.2 | txn BVN1 age 12 | 0 | 0 | 0 | 13 |
