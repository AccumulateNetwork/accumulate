# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 1902 timed reads, p50 1.4 ms, p95 26.1 ms, p99 272.6 ms, **max 822.1 ms** (txn read, BVN1, entry 151 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 192 | 1.4 | 57.5 | 158.1 |
| 100–1000 | 1710 | 1.4 | 14.5 | 822.1 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 951 | 0.7 | 2.1 | 350.6 |
| txn | 951 | 1.8 | 85.6 | 822.1 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 822.1 | txn | BVN1 | 151 |
| 627.7 | txn | BVN1 | 183 |
| 595.7 | txn | Directory | 133 |
| 501.0 | txn | Directory | 133 |
| 468.1 | txn | Directory | 193 |
| 430.7 | txn | BVN2 | 155 |
| 427.8 | txn | Directory | 173 |
| 421.3 | txn | BVN2 | 141 |
| 410.8 | txn | BVN1 | 183 |
| 395.5 | txn | Directory | 173 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 22:37:29 | 6 | 1.7 | 2.0 | 2.0 | txn Directory age 8 | 0 | 0 | 0 | 11 |
| 22:38:30 | 24 | 1.3 | 83.8 | 132.5 | chain BVN1 age 31 | 0 | 0 | 0 | 33 |
| 22:39:30 | 36 | 1.7 | 84.8 | 111.4 | txn BVN2 age 51 | 0 | 0 | 0 | 53 |
| 22:40:31 | 54 | 1.5 | 101.5 | 158.1 | txn Directory age 73 | 0 | 0 | 0 | 73 |
| 22:41:31 | 72 | 1.5 | 2.5 | 56.2 | txn BVN1 age 89 | 0 | 0 | 0 | 93 |
| 22:42:31 | 90 | 1.5 | 3.5 | 91.7 | txn Directory age 113 | 0 | 0 | 0 | 113 |
| 22:43:35 | 108 | 1.7 | 224.1 | 595.7 | txn Directory age 133 | 0 | 0 | 0 | 133 |
| 22:44:35 | 126 | 1.9 | 185.9 | 822.1 | txn BVN1 age 151 | 0 | 0 | 0 | 153 |
| 22:45:36 | 144 | 2.1 | 144.6 | 430.7 | txn BVN2 age 155 | 0 | 0 | 0 | 173 |
| 22:46:36 | 162 | 1.9 | 121.2 | 627.7 | txn BVN1 age 183 | 0 | 0 | 0 | 193 |
| 22:47:33 | 180 | 1.5 | 3.4 | 5.0 | txn Directory age 213 | 0 | 0 | 0 | 213 |
| 22:48:33 | 198 | 1.1 | 1.9 | 5.1 | txn Directory age 231 | 0 | 0 | 0 | 231 |
| 22:49:34 | 216 | 1.4 | 1.9 | 3.1 | txn BVN1 age 194 | 0 | 0 | 0 | 251 |
| 22:50:34 | 234 | 1.5 | 2.1 | 3.7 | txn BVN1 age 194 | 0 | 0 | 0 | 270 |
| 22:51:35 | 252 | 1.3 | 1.7 | 4.5 | txn BVN2 age 185 | 0 | 0 | 0 | 275 |
