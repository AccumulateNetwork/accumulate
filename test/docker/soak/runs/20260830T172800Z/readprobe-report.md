# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 5488 timed reads, p50 1.7 ms, p95 33.9 ms, p99 133.3 ms, **max 4444.9 ms** (txn read, BVN2, entry 402 blocks old); 0 failed, 0 timed out (8s), 26 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 210 | 1.3 | 2.0 | 59.4 |
| 100–1000 | 5278 | 1.7 | 35.8 | 4444.9 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 2746 | 0.9 | 13.1 | 667.3 |
| txn | 2742 | 2.1 | 53.3 | 4444.9 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 4444.9 | txn | BVN2 | 402 |
| 3023.1 | txn | BVN1 | 529 |
| 2839.3 | txn | Directory | 356 |
| 2583.9 | txn | BVN1 | 529 |
| 2318.3 | txn | Directory | 536 |
| 2096.9 | txn | BVN2 | 509 |
| 1817.9 | txn | Directory | 416 |
| 1050.7 | txn | BVN2 | 394 |
| 1002.4 | txn | BVN1 | 403 |
| 902.3 | txn | BVN2 | 447 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 17:29:52 | 6 | 1.4 | 1.7 | 1.7 | txn Directory age 8 | 0 | 0 | 0 | 8 |
| 17:30:52 | 24 | 1.1 | 1.7 | 2.1 | txn BVN1 age 33 | 0 | 0 | 0 | 34 |
| 17:31:53 | 42 | 1.6 | 2.6 | 6.2 | txn Directory age 54 | 0 | 0 | 0 | 54 |
| 17:32:53 | 60 | 1.7 | 2.1 | 59.4 | chain BVN1 age 70 | 0 | 0 | 0 | 74 |
| 17:33:54 | 78 | 1.6 | 1.9 | 1.9 | txn BVN1 age 92 | 0 | 0 | 0 | 94 |
| 17:34:54 | 96 | 1.6 | 2.5 | 4.5 | chain BVN1 age 112 | 0 | 0 | 0 | 114 |
| 17:35:55 | 114 | 1.4 | 2.4 | 98.1 | chain BVN2 age 133 | 0 | 0 | 0 | 134 |
| 17:36:56 | 132 | 1.7 | 5.0 | 86.4 | txn BVN2 age 152 | 0 | 0 | 0 | 154 |
| 17:37:58 | 150 | 1.4 | 2.7 | 10.6 | txn BVN2 age 173 | 0 | 0 | 0 | 175 |
| 17:38:57 | 168 | 1.2 | 2.0 | 262.2 | txn BVN1 age 192 | 0 | 0 | 0 | 194 |
| 17:39:58 | 186 | 1.7 | 3.8 | 264.5 | txn BVN2 age 213 | 0 | 0 | 0 | 215 |
| 17:40:58 | 198 | 1.6 | 3.9 | 5.4 | txn BVN1 age 232 | 0 | 0 | 0 | 235 |
| 17:42:00 | 216 | 1.8 | 3.6 | 447.7 | txn Directory age 255 | 0 | 2 | 0 | 255 |
| 17:43:00 | 234 | 2.6 | 6.0 | 13.1 | txn BVN2 age 272 | 0 | 0 | 0 | 275 |
| 17:44:01 | 252 | 1.9 | 6.9 | 298.0 | chain BVN2 age 290 | 0 | 0 | 0 | 295 |
| 17:45:02 | 270 | 1.1 | 5.1 | 537.7 | txn Directory age 315 | 0 | 0 | 0 | 315 |
| 17:46:02 | 288 | 1.4 | 2.0 | 81.0 | txn BVN1 age 333 | 0 | 0 | 0 | 336 |
| 17:47:06 | 300 | 1.6 | 5.5 | 2839.3 | txn Directory age 356 | 0 | 0 | 0 | 356 |
| 17:48:05 | 300 | 1.9 | 4.9 | 90.7 | txn Directory age 376 | 0 | 0 | 0 | 376 |
| 17:49:06 | 300 | 1.8 | 4.4 | 1050.7 | txn BVN2 age 394 | 0 | 0 | 0 | 396 |
| 17:50:21 | 300 | 12.5 | 141.4 | 4444.9 | txn BVN2 age 402 | 0 | 10 | 0 | 416 |
| 17:51:05 | 300 | 1.6 | 4.0 | 77.1 | txn BVN1 age 433 | 0 | 0 | 0 | 436 |
| 17:52:08 | 300 | 1.3 | 3.7 | 902.3 | txn BVN2 age 447 | 0 | 0 | 0 | 456 |
| 17:53:08 | 300 | 1.8 | 4.9 | 74.0 | txn BVN1 age 472 | 0 | 0 | 0 | 477 |
| 17:54:09 | 300 | 1.8 | 4.7 | 263.0 | txn BVN1 age 493 | 0 | 0 | 0 | 497 |
| 17:55:13 | 300 | 2.6 | 10.5 | 2096.9 | txn BVN2 age 509 | 0 | 12 | 0 | 517 |
| 17:56:30 | 300 | 26.9 | 186.4 | 3023.1 | txn BVN1 age 529 | 0 | 2 | 0 | 536 |
