# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 4350 timed reads, p50 1.1 ms, p95 2.4 ms, p99 4.4 ms, **max 13.0 ms** (txn read, Directory, entry 457 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 30 | 1.9 | 7.7 | 11.4 |
| 100–1000 | 4320 | 1.1 | 2.4 | 13.0 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 2175 | 0.7 | 1.4 | 8.6 |
| txn | 2175 | 1.6 | 2.8 | 13.0 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 13.0 | txn | Directory | 457 |
| 12.0 | txn | Directory | 278 |
| 11.4 | txn | BVN1 | 95 |
| 8.6 | chain | BVN1 | 213 |
| 8.5 | chain | Directory | 457 |
| 8.4 | txn | BVN1 | 450 |
| 7.8 | chain | Directory | 528 |
| 7.8 | chain | BVN2 | 452 |
| 7.7 | txn | BVN2 | 96 |
| 7.4 | chain | Directory | 528 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 00:13:14 | 6 | 1.5 | 2.0 | 2.0 | txn BVN1 age 34 | 0 | 0 | 0 | 38 |
| 00:14:15 | 24 | 2.5 | 7.7 | 11.4 | txn BVN1 age 95 | 0 | 0 | 0 | 99 |
| 00:15:15 | 42 | 2.0 | 4.1 | 7.3 | txn BVN1 age 154 | 0 | 0 | 0 | 159 |
| 00:16:16 | 60 | 1.6 | 2.2 | 8.6 | chain BVN1 age 213 | 0 | 0 | 0 | 219 |
| 00:17:16 | 78 | 1.6 | 4.7 | 12.0 | txn Directory age 278 | 0 | 0 | 0 | 278 |
| 00:18:17 | 96 | 2.0 | 5.4 | 7.2 | txn BVN1 age 332 | 0 | 0 | 0 | 338 |
| 00:19:17 | 114 | 1.8 | 4.0 | 7.0 | txn Directory age 398 | 0 | 0 | 0 | 398 |
| 00:20:18 | 132 | 2.0 | 5.5 | 13.0 | txn Directory age 457 | 0 | 0 | 0 | 457 |
| 00:21:18 | 150 | 1.2 | 2.2 | 4.8 | txn BVN1 age 511 | 0 | 0 | 0 | 517 |
| 00:22:19 | 168 | 1.1 | 2.5 | 7.4 | chain Directory age 528 | 0 | 0 | 0 | 528 |
| 00:23:19 | 186 | 1.3 | 2.4 | 3.8 | txn BVN2 age 524 | 0 | 0 | 0 | 528 |
| 00:24:20 | 204 | 1.2 | 2.5 | 5.4 | txn BVN1 age 522 | 0 | 0 | 0 | 528 |
| 00:25:21 | 222 | 1.2 | 2.4 | 7.1 | txn BVN2 age 524 | 0 | 0 | 0 | 528 |
| 00:26:21 | 240 | 1.2 | 2.4 | 7.8 | chain Directory age 528 | 0 | 0 | 0 | 528 |
| 00:27:22 | 258 | 1.1 | 2.4 | 3.8 | txn Directory age 528 | 0 | 0 | 0 | 528 |
| 00:28:23 | 276 | 1.1 | 1.8 | 2.5 | txn Directory age 528 | 0 | 0 | 0 | 528 |
| 00:29:23 | 294 | 1.0 | 2.0 | 2.7 | txn Directory age 528 | 0 | 0 | 0 | 528 |
| 00:30:24 | 300 | 1.1 | 2.3 | 3.3 | txn Directory age 528 | 0 | 0 | 0 | 528 |
| 00:31:25 | 300 | 1.1 | 2.4 | 5.8 | txn Directory age 528 | 0 | 0 | 0 | 528 |
| 00:32:25 | 300 | 1.1 | 2.0 | 2.7 | txn Directory age 528 | 0 | 0 | 0 | 528 |
| 00:33:26 | 300 | 1.0 | 2.2 | 4.8 | chain BVN1 age 522 | 0 | 0 | 0 | 528 |
| 00:34:27 | 300 | 1.1 | 1.9 | 5.5 | txn Directory age 528 | 0 | 0 | 0 | 528 |
| 00:35:27 | 300 | 1.0 | 2.1 | 5.4 | txn Directory age 528 | 0 | 0 | 0 | 528 |
