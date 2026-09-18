# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 300 timed reads, p50 1.8 ms, p95 168.5 ms, p99 308.9 ms, **max 576.9 ms** (txn read, BVN2, entry 111 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 210 | 1.8 | 173.8 | 317.6 |
| 100–1000 | 90 | 2.2 | 77.8 | 576.9 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 150 | 0.9 | 2.1 | 185.5 |
| txn | 150 | 2.5 | 186.1 | 576.9 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 576.9 | txn | BVN2 | 111 |
| 317.6 | txn | BVN2 | 90 |
| 308.9 | txn | BVN2 | 90 |
| 294.1 | txn | BVN1 | 87 |
| 260.7 | txn | Directory | 93 |
| 222.3 | txn | Directory | 93 |
| 215.0 | txn | BVN2 | 90 |
| 186.1 | txn | BVN1 | 33 |
| 185.5 | chain | BVN2 | 90 |
| 183.4 | txn | BVN1 | 87 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 12:11:46 | 6 | 1.7 | 1.8 | 1.8 | txn BVN2 age 6 | 0 | 0 | 0 | 8 |
| 12:12:47 | 24 | 2.0 | 168.5 | 186.1 | txn BVN1 age 33 | 0 | 0 | 0 | 33 |
| 12:13:47 | 42 | 1.8 | 3.4 | 6.9 | txn Directory age 53 | 0 | 0 | 0 | 53 |
| 12:14:47 | 60 | 1.8 | 3.2 | 17.4 | txn BVN1 age 71 | 0 | 0 | 0 | 73 |
| 12:15:52 | 78 | 1.9 | 260.7 | 317.6 | txn BVN2 age 90 | 0 | 0 | 0 | 93 |
| 12:16:50 | 90 | 2.2 | 77.8 | 576.9 | txn BVN2 age 111 | 0 | 0 | 0 | 113 |
