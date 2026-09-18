# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 7012 timed reads, p50 1.5 ms, p95 3.4 ms, p99 14.1 ms, **max 8040.5 ms** (txn read, BVN1, entry 1659 blocks old); 49 failed, 42 timed out (8s), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 40 | 1.4 | 2.7 | 3.1 |
| 100–1000 | 3004 | 1.4 | 3.1 | 8040.4 |
| 1000–5000 | 3968 | 1.5 | 3.6 | 8040.5 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 3506 | 0.7 | 1.8 | 8040.3 |
| txn | 3506 | 2.0 | 4.3 | 8040.5 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 8040.5 | txn | BVN1 | 1659 |
| 8040.4 | txn | Directory | 507 |
| 8040.3 | txn | Directory | 1709 |
| 8040.3 | txn | BVN3 | 1736 |
| 8040.3 | txn | BVN1 | 510 |
| 8040.3 | txn | Directory | 1709 |
| 8040.3 | txn | BVN2 | 1766 |
| 8040.3 | chain | BVN2 | 1479 |
| 8040.2 | chain | BVN2 | 1118 |
| 8040.2 | chain | BVN2 | 1118 |

## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|
| 21:58:17 | 8 | 2.5 | 3.1 | 3.1 | txn BVN2 age 33 | 0 | 0 | 0 | 38 |
| 21:59:18 | 32 | 1.4 | 2.5 | 2.6 | txn Directory age 97 | 0 | 0 | 0 | 97 |
| 22:00:18 | 56 | 1.3 | 3.1 | 3.5 | txn BVN3 age 153 | 0 | 0 | 0 | 157 |
| 22:01:18 | 80 | 1.2 | 2.8 | 3.3 | txn BVN2 age 212 | 7 | 0 | 0 | 217 |
| 22:02:19 | 104 | 1.4 | 3.7 | 19.1 | txn Directory age 276 | 0 | 0 | 0 | 276 |
| 22:03:20 | 128 | 1.3 | 3.7 | 32.3 | txn BVN1 age 326 | 0 | 0 | 0 | 336 |
| 22:04:20 | 152 | 1.4 | 2.7 | 3.7 | txn BVN3 age 390 | 0 | 0 | 0 | 392 |
| 22:05:21 | 176 | 1.5 | 3.5 | 11.7 | txn BVN1 age 444 | 0 | 0 | 0 | 450 |
| 22:07:48 | 200 | 1.8 | 5862.2 | 8040.4 | txn Directory age 507 | 9 | 0 | 9 | 517 |
| 22:07:49 | 208 | 1.6 | 2.9 | 35.4 | txn BVN3 age 596 | 0 | 0 | 0 | 596 |
| 22:08:50 | 232 | 1.5 | 3.1 | 14.7 | txn BVN1 age 648 | 0 | 0 | 0 | 655 |
| 22:09:50 | 256 | 1.4 | 2.8 | 6.1 | txn BVN3 age 715 | 0 | 0 | 0 | 715 |
| 22:10:58 | 280 | 1.8 | 5.8 | 22.0 | txn Directory age 752 | 0 | 0 | 0 | 781 |
| 22:11:52 | 300 | 1.6 | 3.0 | 11.8 | txn BVN1 age 811 | 0 | 0 | 0 | 834 |
| 22:12:53 | 300 | 1.4 | 3.0 | 14.1 | txn Directory age 864 | 0 | 0 | 0 | 893 |
| 22:13:54 | 300 | 1.5 | 3.1 | 11.5 | txn Directory age 921 | 0 | 0 | 0 | 953 |
| 22:14:54 | 300 | 1.4 | 2.2 | 14.0 | chain BVN2 age 991 | 0 | 0 | 0 | 1013 |
| 22:15:55 | 300 | 1.4 | 3.2 | 15.3 | txn BVN1 age 1050 | 0 | 0 | 0 | 1073 |
| 22:17:48 | 300 | 1.6 | 4.3 | 8040.2 | chain BVN2 age 1118 | 5 | 0 | 5 | 1140 |
| 22:17:56 | 300 | 1.5 | 3.4 | 9.1 | txn BVN3 age 1191 | 0 | 0 | 0 | 1191 |
| 22:18:57 | 300 | 1.5 | 3.3 | 13.9 | txn Directory age 1201 | 0 | 0 | 0 | 1251 |
| 22:19:58 | 300 | 1.5 | 2.9 | 16.6 | chain Directory age 1261 | 0 | 0 | 0 | 1311 |
| 22:20:58 | 300 | 1.5 | 3.0 | 21.7 | txn BVN3 age 1370 | 0 | 0 | 0 | 1370 |
| 22:21:59 | 300 | 1.6 | 3.1 | 21.2 | txn BVN2 age 1409 | 0 | 0 | 0 | 1430 |
| 22:24:49 | 300 | 1.6 | 7.7 | 8040.3 | chain BVN2 age 1479 | 12 | 0 | 12 | 1488 |
| 22:24:51 | 300 | 1.7 | 4.7 | 25.8 | chain Directory age 1533 | 0 | 0 | 0 | 1578 |
| 22:25:52 | 300 | 1.4 | 2.8 | 7.0 | chain BVN2 age 1638 | 0 | 0 | 0 | 1638 |
| 22:26:53 | 300 | 1.5 | 2.7 | 9.2 | txn Directory age 1652 | 0 | 0 | 0 | 1698 |
| 22:30:10 | 300 | 1.7 | 8004.3 | 8040.5 | txn BVN1 age 1659 | 16 | 0 | 16 | 1766 |
| 22:30:12 | 300 | 1.8 | 4.8 | 20.9 | txn BVN2 age 1893 | 0 | 0 | 0 | 1893 |
