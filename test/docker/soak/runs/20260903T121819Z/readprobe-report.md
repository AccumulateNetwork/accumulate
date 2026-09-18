# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 1620 timed reads, p50 75.5 ms, p95 628.0 ms, p99 1210.6 ms, **max 8040.4 ms** (chain read, Directory, entry 637 blocks old); 12 failed, 12 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 22 | 14.6 | 229.0 | 250.4 |
| 100–1000 | 1598 | 80.0 | 636.9 | 8040.4 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 810 | 3.5 | 305.3 | 8040.4 |
| txn | 810 | 239.5 | 746.8 | 8040.3 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 8040.4 | chain | Directory | 637 |
| 8040.3 | chain | BVN2 | 561 |
| 8040.3 | txn | BVN1 | 629 |
| 8040.3 | chain | BVN1 | 629 |
| 8040.3 | chain | BVN2 | 561 |
| 8040.1 | chain | BVN2 | 561 |
| 8040.1 | chain | Directory | 637 |
| 8040.0 | chain | BVN1 | 629 |
| 8039.9 | chain | BVN1 | 629 |
| 8039.9 | chain | BVN2 | 561 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 12:21:06 | 6 | 1.6 | 1.9 | 1.9 | txn BVN1 age 15 | 0 | 0 | 0 | 19 |
| 12:22:09 | 24 | 41.3 | 250.4 | 293.5 | txn Directory age 101 | 0 | 0 | 0 | 101 |
| 12:23:14 | 36 | 91.9 | 564.5 | 746.8 | txn Directory age 161 | 0 | 0 | 0 | 161 |
| 12:24:16 | 54 | 54.1 | 453.8 | 552.7 | txn BVN1 age 203 | 0 | 0 | 0 | 221 |
| 12:25:24 | 72 | 114.2 | 585.0 | 2530.6 | txn BVN2 age 236 | 0 | 0 | 0 | 281 |
| 12:26:30 | 90 | 199.5 | 745.0 | 848.0 | txn BVN1 age 322 | 0 | 0 | 0 | 335 |
| 12:27:33 | 102 | 125.3 | 621.7 | 2054.5 | chain BVN1 age 362 | 0 | 0 | 0 | 399 |
| 12:28:40 | 114 | 166.1 | 795.0 | 1176.2 | txn BVN1 age 443 | 0 | 0 | 0 | 458 |
| 12:29:32 | 126 | 80.8 | 619.4 | 881.8 | txn BVN2 age 440 | 0 | 0 | 0 | 515 |
| 12:30:41 | 138 | 152.4 | 640.3 | 962.8 | txn BVN1 age 526 | 0 | 0 | 0 | 556 |
| 12:33:16 | 150 | 164.3 | 8040.0 | 8040.4 | chain Directory age 637 | 12 | 0 | 12 | 637 |
| 12:33:41 | 156 | 56.6 | 487.7 | 1144.7 | txn Directory age 653 | 0 | 0 | 0 | 667 |
| 12:34:36 | 168 | 28.0 | 395.8 | 1395.5 | chain BVN1 age 629 | 0 | 0 | 0 | 695 |
| 12:35:44 | 186 | 8.9 | 510.7 | 967.8 | txn BVN1 age 763 | 0 | 0 | 0 | 763 |
| 12:36:43 | 198 | 19.7 | 535.5 | 1210.6 | txn BVN2 age 670 | 0 | 0 | 0 | 751 |
