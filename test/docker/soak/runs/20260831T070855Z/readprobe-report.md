# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 2850 timed reads, p50 1.3 ms, p95 2.6 ms, p99 6.1 ms, **max 158.6 ms** (chain read, Directory, entry 335 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 210 | 1.2 | 3.4 | 8.5 |
| 100–1000 | 2640 | 1.3 | 2.6 | 158.6 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 1425 | 0.7 | 1.2 | 158.6 |
| txn | 1425 | 1.8 | 3.4 | 83.7 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 158.6 | chain | Directory | 335 |
| 83.7 | txn | BVN2 | 277 |
| 72.0 | chain | BVN2 | 266 |
| 69.1 | txn | BVN2 | 251 |
| 63.6 | txn | BVN1 | 233 |
| 60.4 | txn | BVN2 | 192 |
| 59.3 | txn | Directory | 235 |
| 53.4 | txn | BVN1 | 253 |
| 37.8 | txn | BVN2 | 192 |
| 31.6 | txn | BVN1 | 192 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 07:11:27 | 6 | 1.5 | 1.8 | 1.8 | txn Directory age 8 | 0 | 0 | 0 | 8 |
| 07:12:28 | 24 | 1.0 | 1.3 | 1.3 | txn BVN1 age 33 | 0 | 0 | 0 | 34 |
| 07:13:28 | 42 | 1.5 | 3.1 | 8.2 | txn BVN1 age 51 | 0 | 0 | 0 | 54 |
| 07:14:29 | 60 | 1.6 | 2.2 | 4.7 | txn BVN1 age 72 | 0 | 0 | 0 | 74 |
| 07:15:29 | 78 | 1.6 | 4.1 | 8.5 | txn Directory age 94 | 0 | 0 | 0 | 94 |
| 07:16:30 | 96 | 1.6 | 2.4 | 3.7 | txn BVN1 age 112 | 0 | 0 | 0 | 114 |
| 07:17:30 | 114 | 1.7 | 5.2 | 14.3 | txn Directory age 134 | 0 | 0 | 0 | 134 |
| 07:18:31 | 132 | 1.4 | 2.1 | 5.2 | chain BVN1 age 152 | 0 | 0 | 0 | 154 |
| 07:19:31 | 150 | 1.3 | 2.3 | 4.2 | chain BVN2 age 172 | 0 | 0 | 0 | 174 |
| 07:20:32 | 168 | 1.4 | 3.2 | 60.4 | txn BVN2 age 192 | 0 | 0 | 0 | 194 |
| 07:21:33 | 186 | 1.5 | 2.3 | 5.6 | txn Directory age 214 | 0 | 0 | 0 | 214 |
| 07:22:33 | 204 | 1.6 | 2.3 | 63.6 | txn BVN1 age 233 | 0 | 0 | 0 | 235 |
| 07:23:34 | 222 | 1.8 | 3.8 | 69.1 | txn BVN2 age 251 | 0 | 0 | 0 | 255 |
| 07:24:35 | 240 | 1.8 | 4.7 | 72.0 | chain BVN2 age 266 | 0 | 0 | 0 | 275 |
| 07:25:36 | 258 | 1.2 | 1.8 | 2.1 | txn BVN1 age 294 | 0 | 0 | 0 | 295 |
| 07:26:36 | 276 | 1.2 | 2.0 | 3.1 | txn BVN1 age 314 | 0 | 0 | 0 | 315 |
| 07:27:37 | 294 | 1.1 | 2.8 | 158.6 | chain Directory age 335 | 0 | 0 | 0 | 335 |
| 07:28:38 | 300 | 1.0 | 1.7 | 4.1 | txn Directory age 342 | 0 | 0 | 0 | 354 |
