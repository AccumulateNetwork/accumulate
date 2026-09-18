# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 2778 timed reads, p50 3.0 ms, p95 17.0 ms, p99 40.9 ms, **max 88.5 ms** (txn read, BVN1, entry 740 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 30 | 1.4 | 2.6 | 2.7 |
| 100–1000 | 2648 | 3.1 | 17.5 | 88.5 |
| 1000–5000 | 100 | 2.4 | 6.1 | 9.2 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 1389 | 1.3 | 8.1 | 86.0 |
| txn | 1389 | 4.6 | 22.0 | 88.5 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 88.5 | txn | BVN1 | 740 |
| 86.0 | chain | BVN1 | 391 |
| 79.4 | chain | BVN2 | 262 |
| 63.0 | chain | BVN1 | 509 |
| 62.2 | txn | BVN2 | 402 |
| 59.9 | txn | Directory | 256 |
| 58.9 | txn | Directory | 426 |
| 58.5 | txn | BVN1 | 450 |
| 57.1 | txn | Directory | 560 |
| 53.4 | txn | BVN2 | 559 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 14:03:27 | 6 | 1.8 | 2.7 | 2.7 | txn BVN1 age 7 | 0 | 0 | 0 | 10 |
| 14:04:28 | 24 | 1.4 | 2.6 | 2.6 | txn Directory age 98 | 0 | 0 | 0 | 98 |
| 14:05:28 | 42 | 3.2 | 16.2 | 35.1 | txn Directory age 156 | 0 | 0 | 0 | 156 |
| 14:06:29 | 60 | 2.9 | 12.4 | 22.0 | txn BVN2 age 209 | 0 | 0 | 0 | 212 |
| 14:07:30 | 78 | 4.6 | 45.3 | 79.4 | chain BVN2 age 262 | 0 | 0 | 0 | 271 |
| 14:08:30 | 90 | 3.3 | 23.8 | 48.9 | txn BVN1 age 330 | 0 | 0 | 0 | 330 |
| 14:09:31 | 108 | 4.1 | 19.9 | 86.0 | chain BVN1 age 391 | 0 | 0 | 0 | 391 |
| 14:10:31 | 126 | 4.9 | 33.4 | 62.2 | txn BVN2 age 402 | 0 | 0 | 0 | 450 |
| 14:11:32 | 144 | 4.0 | 30.5 | 63.0 | chain BVN1 age 509 | 0 | 0 | 0 | 509 |
| 14:12:33 | 162 | 5.1 | 19.6 | 58.9 | txn Directory age 426 | 0 | 0 | 0 | 568 |
| 14:13:33 | 180 | 4.4 | 21.6 | 51.8 | txn BVN1 age 624 | 0 | 0 | 0 | 624 |
| 14:14:33 | 198 | 2.9 | 10.7 | 45.1 | txn BVN2 age 523 | 0 | 0 | 0 | 674 |
| 14:15:34 | 216 | 3.1 | 28.0 | 88.5 | txn BVN1 age 740 | 0 | 0 | 0 | 740 |
| 14:16:34 | 234 | 2.7 | 11.6 | 57.1 | txn Directory age 560 | 0 | 0 | 0 | 799 |
| 14:17:34 | 252 | 3.4 | 9.8 | 21.9 | txn Directory age 560 | 0 | 0 | 0 | 851 |
| 14:18:34 | 270 | 2.3 | 5.5 | 12.7 | txn BVN2 age 651 | 0 | 0 | 0 | 919 |
| 14:19:36 | 288 | 4.0 | 15.5 | 31.8 | txn BVN1 age 978 | 0 | 0 | 0 | 978 |
| 14:20:36 | 300 | 2.3 | 5.8 | 13.8 | txn Directory age 603 | 0 | 0 | 0 | 1038 |
