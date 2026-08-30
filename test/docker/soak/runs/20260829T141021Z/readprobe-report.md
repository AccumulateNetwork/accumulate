# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 2784 timed reads, p50 1.0 ms, p95 1.9 ms, p99 2.7 ms, **max 210.8 ms** (txn read, BVN2, entry 183 blocks old); 78 failed.

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 888 | 0.9 | 2.0 | 6.7 |
| 100–1000 | 1896 | 1.0 | 1.9 | 210.8 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 1392 | 0.6 | 1.3 | 2.5 |
| txn | 1392 | 1.3 | 2.2 | 210.8 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 210.8 | txn | BVN2 | 183 |
| 111.3 | txn | Directory | 104 |
| 32.0 | txn | BVN1 | 204 |
| 6.7 | txn | BVN2 | 72 |
| 5.9 | txn | BVN1 | 183 |
| 5.1 | txn | BVN1 | 204 |
| 4.9 | txn | BVN2 | 163 |
| 4.2 | txn | Directory | 185 |
| 3.6 | txn | Directory | 185 |
| 3.5 | txn | Directory | 64 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | oldest in reservoir |
|---|---|---|---|---|---|---|---|
| 14:12:06 | 6 | 1.4 | 1.9 | 1.9 | txn BVN1 age 6 | 0 | 8 |
| 14:13:07 | 24 | 1.1 | 1.5 | 1.6 | txn BVN1 age 33 | 0 | 34 |
| 14:14:07 | 42 | 1.3 | 2.4 | 2.9 | txn BVN2 age 53 | 0 | 54 |
| 14:15:07 | 60 | 1.2 | 2.7 | 6.7 | txn BVN2 age 72 | 0 | 74 |
| 14:16:07 | 78 | 1.1 | 1.8 | 2.1 | txn BVN1 age 92 | 0 | 94 |
| 14:17:08 | 78 | 0.0 | 0.1 | 0.1 | txn Directory age 94 | 78 | 94 |
| 14:18:08 | 84 | 0.9 | 1.8 | 3.0 | txn BVN1 age 2 | 0 | 4 |
| 14:19:08 | 102 | 0.9 | 1.9 | 2.5 | chain BVN2 age 23 | 0 | 24 |
| 14:20:08 | 120 | 0.9 | 1.8 | 2.7 | txn BVN2 age 43 | 0 | 44 |
| 14:21:08 | 138 | 1.0 | 2.1 | 3.5 | txn Directory age 64 | 0 | 64 |
| 14:22:09 | 156 | 0.9 | 2.0 | 2.7 | txn BVN1 age 83 | 0 | 84 |
| 14:23:10 | 174 | 1.0 | 2.0 | 111.3 | txn Directory age 104 | 0 | 104 |
| 14:24:10 | 192 | 1.0 | 2.2 | 2.9 | txn Directory age 125 | 0 | 125 |
| 14:25:11 | 210 | 1.1 | 1.7 | 3.1 | txn Directory age 145 | 0 | 145 |
| 14:26:11 | 228 | 1.1 | 1.9 | 4.9 | txn BVN2 age 163 | 0 | 165 |
| 14:27:11 | 246 | 1.2 | 1.9 | 210.8 | txn BVN2 age 183 | 0 | 185 |
| 14:28:12 | 264 | 1.0 | 2.0 | 32.0 | txn BVN1 age 204 | 0 | 205 |
| 14:29:13 | 282 | 1.0 | 1.9 | 3.4 | txn BVN2 age 224 | 0 | 225 |
| 14:30:13 | 300 | 0.9 | 1.7 | 2.8 | txn BVN1 age 244 | 0 | 245 |
