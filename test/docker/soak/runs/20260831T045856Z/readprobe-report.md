# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 1722 timed reads, p50 1.2 ms, p95 3.0 ms, p99 5.7 ms, **max 80.7 ms** (chain read, BVN1, entry 193 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 210 | 1.0 | 2.1 | 3.6 |
| 100–1000 | 1512 | 1.2 | 3.1 | 80.7 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 861 | 0.7 | 1.4 | 80.7 |
| txn | 861 | 1.9 | 3.4 | 62.9 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 80.7 | chain | BVN1 | 193 |
| 62.9 | txn | BVN1 | 252 |
| 53.9 | txn | Directory | 275 |
| 44.7 | txn | BVN1 | 232 |
| 38.0 | txn | Directory | 255 |
| 32.3 | txn | BVN2 | 232 |
| 31.8 | txn | BVN1 | 232 |
| 16.4 | txn | BVN1 | 232 |
| 12.0 | txn | BVN2 | 271 |
| 8.4 | txn | BVN2 | 232 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 05:01:10 | 6 | 1.4 | 1.7 | 1.7 | txn Directory age 8 | 0 | 0 | 0 | 8 |
| 05:02:10 | 24 | 0.9 | 1.6 | 1.6 | txn Directory age 34 | 0 | 0 | 0 | 34 |
| 05:03:10 | 42 | 1.2 | 1.5 | 2.5 | txn Directory age 54 | 0 | 0 | 0 | 54 |
| 05:04:11 | 60 | 1.2 | 2.0 | 3.1 | txn Directory age 74 | 0 | 0 | 0 | 74 |
| 05:05:11 | 78 | 1.0 | 2.5 | 3.6 | chain BVN1 age 93 | 0 | 0 | 0 | 94 |
| 05:06:12 | 96 | 1.4 | 2.0 | 3.5 | txn BVN1 age 112 | 0 | 0 | 0 | 114 |
| 05:07:12 | 114 | 1.1 | 2.2 | 4.2 | txn Directory age 134 | 0 | 0 | 0 | 134 |
| 05:08:13 | 132 | 1.4 | 2.2 | 3.7 | txn Directory age 154 | 0 | 0 | 0 | 154 |
| 05:09:13 | 150 | 1.1 | 1.6 | 4.0 | txn BVN2 age 172 | 0 | 0 | 0 | 174 |
| 05:10:14 | 168 | 1.1 | 1.6 | 80.7 | chain BVN1 age 193 | 0 | 0 | 0 | 194 |
| 05:11:14 | 186 | 1.5 | 3.1 | 4.2 | txn BVN2 age 212 | 0 | 0 | 0 | 214 |
| 05:12:15 | 204 | 2.0 | 3.7 | 44.7 | txn BVN1 age 232 | 0 | 0 | 0 | 234 |
| 05:13:16 | 222 | 1.8 | 2.9 | 62.9 | txn BVN1 age 252 | 0 | 0 | 0 | 255 |
| 05:14:17 | 240 | 2.2 | 3.7 | 53.9 | txn Directory age 275 | 0 | 0 | 0 | 275 |
