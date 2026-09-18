# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 2514 timed reads, p50 1.7 ms, p95 6.0 ms, p99 21.1 ms, **max 162.5 ms** (txn read, BVN1, entry 618 blocks old); 0 failed, 0 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 30 | 1.5 | 6.7 | 7.4 |
| 100–1000 | 2484 | 1.7 | 5.9 | 162.5 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 1257 | 0.8 | 2.3 | 31.9 |
| txn | 1257 | 2.4 | 9.3 | 162.5 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 162.5 | txn | BVN1 | 618 |
| 47.5 | txn | Directory | 635 |
| 37.2 | txn | Directory | 752 |
| 37.0 | txn | BVN2 | 614 |
| 31.9 | chain | BVN2 | 614 |
| 31.6 | txn | BVN2 | 614 |
| 30.7 | txn | BVN1 | 721 |
| 30.1 | txn | BVN2 | 716 |
| 29.9 | txn | BVN1 | 567 |
| 29.7 | txn | BVN2 | 614 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 20:29:06 | 6 | 1.6 | 1.6 | 1.6 | txn Directory age 15 | 0 | 0 | 0 | 15 |
| 20:30:07 | 24 | 1.5 | 6.7 | 7.4 | txn Directory age 96 | 0 | 0 | 0 | 96 |
| 20:31:07 | 42 | 1.2 | 1.8 | 1.9 | txn BVN2 age 152 | 0 | 0 | 0 | 156 |
| 20:32:07 | 60 | 1.6 | 2.0 | 2.8 | txn BVN2 age 211 | 0 | 0 | 0 | 216 |
| 20:33:08 | 78 | 1.6 | 2.3 | 3.3 | chain Directory age 276 | 0 | 0 | 0 | 276 |
| 20:34:08 | 96 | 1.3 | 1.9 | 4.2 | txn Directory age 336 | 0 | 0 | 0 | 336 |
| 20:35:09 | 114 | 2.1 | 12.1 | 18.0 | chain BVN2 age 389 | 0 | 0 | 0 | 395 |
| 20:36:09 | 132 | 1.6 | 3.0 | 10.3 | chain BVN2 age 446 | 0 | 0 | 0 | 456 |
| 20:37:10 | 150 | 1.6 | 2.7 | 15.1 | txn BVN2 age 508 | 0 | 0 | 0 | 515 |
| 20:38:11 | 168 | 2.2 | 5.8 | 29.9 | txn BVN1 age 567 | 0 | 0 | 0 | 575 |
| 20:39:12 | 186 | 2.0 | 19.1 | 162.5 | txn BVN1 age 618 | 0 | 0 | 0 | 635 |
| 20:40:12 | 198 | 1.8 | 9.4 | 19.5 | txn Directory age 695 | 0 | 0 | 0 | 695 |
| 20:41:13 | 216 | 2.1 | 24.2 | 37.2 | txn Directory age 752 | 0 | 0 | 0 | 752 |
| 20:42:13 | 234 | 1.5 | 2.7 | 10.0 | txn Directory age 799 | 0 | 0 | 0 | 799 |
| 20:43:14 | 252 | 1.9 | 3.8 | 30.1 | txn BVN2 age 716 | 0 | 0 | 0 | 849 |
| 20:44:15 | 270 | 2.3 | 4.7 | 16.7 | txn BVN1 age 902 | 0 | 0 | 0 | 909 |
| 20:45:16 | 288 | 2.8 | 6.1 | 20.3 | txn BVN2 age 716 | 0 | 0 | 0 | 967 |
