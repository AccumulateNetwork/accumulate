# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 210 timed reads, p50 1.4 ms, p95 4.3 ms, p99 16.5 ms, **max 28.5 ms** (txn read, BVN2, entry 212 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 30 | 1.5 | 4.0 | 5.3 |
| 100–1000 | 180 | 1.4 | 4.4 | 28.5 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 105 | 0.7 | 1.9 | 16.5 |
| txn | 105 | 1.8 | 4.8 | 28.5 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 28.5 | txn | BVN2 | 212 |
| 23.6 | txn | BVN1 | 212 |
| 16.5 | chain | Directory | 217 |
| 12.2 | txn | BVN1 | 272 |
| 8.3 | chain | Directory | 157 |
| 8.0 | txn | Directory | 217 |
| 5.3 | txn | BVN2 | 93 |
| 4.8 | txn | BVN1 | 153 |
| 4.6 | chain | BVN1 | 272 |
| 4.4 | txn | BVN1 | 272 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 13:17:54 | 6 | 1.8 | 2.4 | 2.4 | txn Directory age 15 | 0 | 0 | 0 | 16 |
| 13:18:55 | 24 | 1.5 | 4.0 | 5.3 | txn BVN2 age 93 | 0 | 0 | 0 | 97 |
| 13:19:55 | 42 | 1.3 | 3.8 | 8.3 | chain Directory age 157 | 0 | 0 | 0 | 157 |
| 13:20:56 | 60 | 1.4 | 16.5 | 28.5 | txn BVN2 age 212 | 0 | 0 | 0 | 217 |
| 13:21:56 | 78 | 1.5 | 3.6 | 12.2 | txn BVN1 age 272 | 0 | 0 | 0 | 277 |
