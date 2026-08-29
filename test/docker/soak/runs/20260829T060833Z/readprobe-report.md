# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 6720 timed reads, p50 1.4 ms, p95 4.2 ms, p99 40.2 ms, **max 3750.5 ms** (txn read, BVN2, entry 550 blocks old); 12 failed.

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 210 | 1.3 | 2.4 | 4.8 |
| 100–1000 | 6510 | 1.4 | 4.2 | 3750.5 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 3360 | 0.7 | 1.8 | 1213.1 |
| txn | 3360 | 2.0 | 5.6 | 3750.5 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 3750.5 | txn | BVN2 | 550 |
| 2214.2 | txn | BVN1 | 495 |
| 1895.9 | txn | BVN1 | 614 |
| 1404.1 | txn | Directory | 557 |
| 1381.2 | txn | Directory | 557 |
| 1344.1 | txn | BVN1 | 314 |
| 1213.1 | chain | BVN2 | 562 |
| 1095.9 | txn | Directory | 436 |
| 1093.8 | txn | BVN2 | 432 |
| 1035.1 | txn | BVN2 | 450 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | oldest in reservoir |
|---|---|---|---|---|---|---|---|
| 06:10:02 | 6 | 1.6 | 1.7 | 1.7 | txn Directory age 8 | 0 | 8 |
| 06:11:02 | 24 | 1.1 | 1.5 | 2.0 | txn BVN2 age 31 | 0 | 34 |
| 06:12:02 | 42 | 1.4 | 1.9 | 3.8 | txn BVN1 age 52 | 0 | 54 |
| 06:13:03 | 60 | 1.6 | 2.3 | 3.2 | chain BVN2 age 72 | 0 | 74 |
| 06:14:04 | 78 | 1.6 | 3.6 | 4.8 | chain BVN2 age 92 | 0 | 94 |
| 06:15:04 | 96 | 1.7 | 2.2 | 3.7 | txn BVN1 age 112 | 0 | 114 |
| 06:16:05 | 114 | 1.1 | 2.1 | 275.8 | txn BVN1 age 132 | 0 | 134 |
| 06:17:05 | 132 | 1.0 | 1.9 | 2.6 | txn BVN2 age 153 | 0 | 154 |
| 06:18:06 | 150 | 1.1 | 2.0 | 3.9 | txn Directory age 174 | 0 | 174 |
| 06:19:07 | 168 | 1.0 | 1.5 | 2.0 | txn BVN2 age 193 | 0 | 195 |
| 06:20:07 | 186 | 1.2 | 1.9 | 91.9 | chain BVN2 age 213 | 0 | 215 |
| 06:21:08 | 204 | 1.5 | 2.3 | 9.0 | chain BVN2 age 233 | 0 | 235 |
| 06:22:08 | 216 | 1.4 | 3.3 | 6.8 | txn BVN2 age 253 | 0 | 255 |
| 06:23:09 | 234 | 1.4 | 2.2 | 89.6 | txn BVN1 age 273 | 0 | 275 |
| 06:24:10 | 252 | 1.5 | 2.8 | 6.4 | txn BVN1 age 294 | 0 | 295 |
| 06:25:13 | 270 | 1.7 | 4.0 | 1344.1 | txn BVN1 age 314 | 0 | 315 |
| 06:26:12 | 288 | 1.4 | 2.5 | 358.6 | txn BVN1 age 334 | 0 | 336 |
| 06:27:15 | 300 | 1.4 | 3.2 | 238.5 | txn BVN2 age 354 | 0 | 356 |
| 06:28:13 | 300 | 1.6 | 2.6 | 378.3 | txn BVN1 age 374 | 0 | 376 |
| 06:29:13 | 300 | 1.3 | 2.8 | 6.5 | txn BVN2 age 394 | 0 | 396 |
| 06:30:15 | 300 | 1.6 | 4.0 | 897.8 | txn BVN2 age 413 | 0 | 416 |
| 06:31:20 | 300 | 1.7 | 7.8 | 1095.9 | txn Directory age 436 | 3 | 436 |
| 06:32:17 | 300 | 1.8 | 6.1 | 1035.1 | txn BVN2 age 450 | 0 | 456 |
| 06:33:16 | 300 | 1.7 | 3.7 | 98.4 | txn BVN2 age 469 | 0 | 476 |
| 06:34:18 | 300 | 2.0 | 4.5 | 2214.2 | txn BVN1 age 495 | 0 | 496 |
| 06:35:17 | 300 | 1.8 | 4.5 | 117.4 | txn Directory age 516 | 0 | 516 |
| 06:36:20 | 300 | 2.2 | 10.3 | 942.2 | txn BVN1 age 535 | 3 | 536 |
| 06:37:21 | 300 | 1.9 | 6.3 | 1404.1 | txn Directory age 557 | 1 | 557 |
| 06:38:18 | 300 | 1.7 | 5.6 | 47.3 | chain BVN2 age 537 | 0 | 577 |
| 06:39:22 | 300 | 1.6 | 3.8 | 3750.5 | txn BVN2 age 550 | 0 | 597 |
| 06:40:23 | 300 | 2.1 | 11.1 | 1895.9 | txn BVN1 age 614 | 5 | 617 |
