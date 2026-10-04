# Read-back probe

Every 20s one recent committed entry per partition joins a reservoir (cap 600); every 60s 150 of them are re-read (chain entry by index, transaction by id) and timed.

**Whole run:** 16098 timed reads, p50 1.2 ms, p95 9.2 ms, p99 56.6 ms, **max 197.4 ms** (txn read, BVN1, entry 2350 blocks old); 0 failed (0 of them timed out, 8s), 278 refused NotReady (a joining node's designed answer; not timed), 0 refused by the API's query gate (not timed).

## Latency by entry age

| age (blocks) | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| 0–100 | 40 | 1.7 | 3.9 | 4.8 |
| 100–1000 | 3086 | 1.3 | 14.5 | 144.5 |
| 1000–5000 | 12972 | 1.2 | 8.3 | 197.4 |

## Latency by read kind

| kind | reads | p50 ms | p95 ms | max ms |
|---|---|---|---|---|
| chain | 8121 | 0.6 | 1.5 | 144.5 |
| txn | 7977 | 1.6 | 24.3 | 197.4 |

## Slowest ten

| ms | kind | partition | age (blocks) |
|---|---|---|---|
| 197.4 | txn | BVN1 | 2350 |
| 196.8 | txn | Directory | 2436 |
| 191.6 | txn | BVN3 | 2297 |
| 184.8 | txn | BVN3 | 2178 |
| 182.8 | txn | BVN1 | 2113 |
| 152.5 | txn | BVN2 | 2116 |
| 147.9 | txn | BVN2 | 3365 |
| 144.5 | chain | BVN3 | 808 |
| 140.8 | txn | BVN2 | 2176 |
| 138.0 | txn | BVN3 | 748 |

## Followers (#4365) — does a node in no committee answer?

**acc-bvn1-fol1** (partitions Directory, BVN1): 2135 reads of entries it holds, **1759 answered**, 376 refused (query gate or NotReady), 0 failed; p50 0.3 ms, p95 45.9 ms, max 151.1 ms.

Read from its own port, never through the router: a read that another node could have answered says nothing about this one. Entries of partitions it does not run are not asked for.

**acc-bvn2-fol1** (partitions Directory, BVN2): 2102 reads of entries it holds, **1742 answered**, 360 refused (query gate or NotReady), 0 failed; p50 0.3 ms, p95 37.9 ms, max 205.6 ms.

Read from its own port, never through the router: a read that another node could have answered says nothing about this one. Entries of partitions it does not run are not asked for.

**acc-bvn3-fol1** (partitions Directory, BVN3): 4088 reads of entries it holds, **4088 answered**, 0 refused (query gate or NotReady), 0 failed; p50 0.3 ms, p95 0.6 ms, max 82.8 ms.

Read from its own port, never through the router: a read that another node could have answered says nothing about this one. Entries of partitions it does not run are not asked for.

**acc-bvn3-fol2** (partitions Directory, BVN3): 2093 reads of entries it holds, **1765 answered**, 328 refused (query gate or NotReady), 0 failed; p50 0.3 ms, p95 43.8 ms, max 147.0 ms.

Read from its own port, never through the router: a read that another node could have answered says nothing about this one. Entries of partitions it does not run are not asked for.


## Rounds

| time | reads | p50 | p95 | max | slowest was | failed | NotReady | gated | timeouts | oldest in reservoir |
|---|---|---|---|---|---|---|---|---|---|---|
| 17:31:14 | 8 | 2.4 | 3.9 | 3.9 | txn Directory age 40 | 0 | 0 | 0 | 0 | 40 |
| 17:32:14 | 32 | 1.6 | 3.1 | 4.8 | txn BVN1 age 95 | 0 | 0 | 0 | 0 | 99 |
| 17:33:14 | 56 | 1.7 | 5.6 | 12.0 | txn BVN3 age 155 | 0 | 0 | 0 | 0 | 159 |
| 17:34:14 | 80 | 2.1 | 4.2 | 30.7 | txn BVN1 age 213 | 0 | 0 | 0 | 0 | 219 |
| 17:35:14 | 104 | 1.6 | 4.3 | 22.4 | txn BVN1 age 272 | 0 | 0 | 0 | 0 | 278 |
| 17:36:15 | 128 | 1.6 | 3.7 | 29.5 | txn Directory age 338 | 0 | 0 | 0 | 0 | 338 |
| 17:37:15 | 152 | 1.6 | 4.4 | 27.6 | txn Directory age 398 | 0 | 0 | 0 | 0 | 398 |
| 17:38:15 | 176 | 1.6 | 4.4 | 12.9 | txn BVN3 age 451 | 0 | 0 | 0 | 0 | 458 |
| 17:39:16 | 200 | 1.7 | 4.3 | 32.0 | txn BVN1 age 509 | 0 | 0 | 0 | 0 | 518 |
| 17:40:16 | 224 | 1.7 | 4.0 | 27.8 | txn BVN3 age 570 | 0 | 0 | 0 | 0 | 578 |
| 17:41:17 | 248 | 1.1 | 1.5 | 24.6 | txn BVN2 age 629 | 0 | 0 | 0 | 0 | 638 |
| 17:42:18 | 272 | 0.8 | 2.2 | 71.3 | txn BVN3 age 689 | 0 | 50 | 0 | 0 | 697 |
| 17:43:20 | 296 | 1.6 | 56.1 | 138.0 | txn BVN3 age 748 | 0 | 0 | 0 | 0 | 757 |
| 17:44:19 | 300 | 1.5 | 40.1 | 144.5 | chain BVN3 age 808 | 0 | 0 | 0 | 0 | 817 |
| 17:45:20 | 300 | 1.4 | 41.5 | 92.2 | txn BVN3 age 867 | 0 | 0 | 0 | 0 | 877 |
| 17:46:19 | 300 | 1.2 | 1.8 | 45.6 | txn BVN2 age 926 | 0 | 0 | 0 | 0 | 937 |
| 17:47:19 | 300 | 1.4 | 3.7 | 39.3 | txn BVN1 age 984 | 0 | 0 | 0 | 0 | 997 |
| 17:48:20 | 300 | 1.2 | 1.8 | 25.2 | txn BVN1 age 1044 | 0 | 0 | 0 | 0 | 1058 |
| 17:49:21 | 300 | 1.2 | 5.5 | 26.6 | txn Directory age 1118 | 0 | 0 | 0 | 0 | 1118 |
| 17:50:21 | 300 | 1.2 | 1.8 | 35.7 | txn BVN2 age 1164 | 0 | 0 | 0 | 0 | 1178 |
| 17:51:22 | 300 | 1.3 | 2.1 | 27.0 | txn BVN3 age 1225 | 0 | 0 | 0 | 0 | 1238 |
| 17:52:22 | 300 | 1.2 | 3.1 | 20.8 | txn BVN2 age 1283 | 0 | 0 | 0 | 0 | 1297 |
| 17:53:23 | 300 | 1.1 | 1.8 | 39.5 | chain BVN3 age 1344 | 0 | 0 | 0 | 0 | 1357 |
| 17:54:23 | 300 | 1.1 | 1.7 | 26.6 | txn BVN3 age 1404 | 0 | 0 | 0 | 0 | 1417 |
| 17:55:24 | 300 | 1.2 | 1.8 | 24.0 | txn Directory age 1477 | 0 | 0 | 0 | 0 | 1477 |
| 17:56:24 | 300 | 1.1 | 1.6 | 22.8 | txn BVN3 age 1523 | 0 | 0 | 0 | 0 | 1537 |
| 17:57:24 | 300 | 1.2 | 1.7 | 23.7 | txn Directory age 1597 | 0 | 0 | 0 | 0 | 1597 |
| 17:58:25 | 300 | 1.1 | 1.6 | 23.5 | txn BVN2 age 1640 | 0 | 0 | 0 | 0 | 1657 |
| 17:59:25 | 300 | 1.2 | 3.7 | 23.5 | chain Directory age 1716 | 0 | 0 | 0 | 0 | 1716 |
| 18:00:26 | 300 | 1.1 | 1.4 | 1.7 | txn Directory age 1777 | 0 | 0 | 0 | 0 | 1777 |
| 18:01:26 | 300 | 1.1 | 1.8 | 26.2 | txn BVN2 age 1818 | 0 | 0 | 0 | 0 | 1836 |
| 18:02:26 | 300 | 1.1 | 1.5 | 2.2 | txn BVN3 age 1880 | 0 | 0 | 0 | 0 | 1897 |
| 18:03:27 | 300 | 1.0 | 3.5 | 22.5 | txn Directory age 1956 | 0 | 41 | 0 | 0 | 1956 |
| 18:04:27 | 300 | 0.8 | 2.1 | 69.0 | txn BVN2 age 1997 | 0 | 55 | 0 | 0 | 2016 |
| 18:05:29 | 300 | 1.4 | 18.2 | 48.3 | txn BVN3 age 2059 | 0 | 15 | 0 | 0 | 2077 |
| 18:06:32 | 300 | 1.6 | 56.2 | 182.8 | txn BVN1 age 2113 | 0 | 0 | 0 | 0 | 2137 |
| 18:07:32 | 300 | 1.6 | 55.7 | 184.8 | txn BVN3 age 2178 | 0 | 0 | 0 | 0 | 2197 |
| 18:08:32 | 300 | 1.6 | 55.2 | 131.5 | chain BVN1 age 2232 | 0 | 0 | 0 | 0 | 2257 |
| 18:09:32 | 300 | 1.6 | 49.5 | 191.6 | txn BVN3 age 2297 | 0 | 0 | 0 | 0 | 2317 |
| 18:10:33 | 300 | 1.6 | 48.3 | 197.4 | txn BVN1 age 2350 | 0 | 0 | 0 | 0 | 2377 |
| 18:11:32 | 300 | 1.4 | 15.4 | 196.8 | txn Directory age 2436 | 0 | 0 | 0 | 0 | 2436 |
| 18:12:32 | 300 | 1.5 | 30.6 | 135.7 | chain BVN2 age 2472 | 0 | 0 | 0 | 0 | 2496 |
| 18:13:32 | 300 | 1.3 | 2.3 | 44.2 | txn BVN3 age 2535 | 0 | 0 | 0 | 0 | 2556 |
| 18:14:32 | 300 | 1.2 | 1.8 | 26.4 | txn BVN3 age 2595 | 0 | 0 | 0 | 0 | 2616 |
| 18:15:33 | 300 | 1.2 | 1.9 | 29.9 | txn BVN3 age 2654 | 0 | 0 | 0 | 0 | 2676 |
| 18:16:33 | 300 | 1.3 | 4.9 | 24.9 | txn BVN2 age 2711 | 0 | 0 | 0 | 0 | 2736 |
| 18:17:34 | 300 | 1.2 | 4.2 | 32.4 | txn BVN1 age 2766 | 0 | 0 | 0 | 0 | 2796 |
| 18:18:34 | 300 | 1.2 | 1.8 | 28.5 | txn BVN2 age 2830 | 0 | 0 | 0 | 0 | 2856 |
| 18:19:35 | 300 | 1.2 | 1.7 | 26.1 | txn BVN3 age 2893 | 0 | 0 | 0 | 0 | 2916 |
| 18:20:35 | 300 | 1.4 | 3.6 | 22.8 | chain BVN2 age 2948 | 0 | 0 | 0 | 0 | 2976 |
| 18:21:36 | 300 | 1.2 | 1.9 | 27.8 | txn Directory age 3037 | 0 | 0 | 0 | 0 | 3037 |
| 18:22:36 | 300 | 1.3 | 1.7 | 30.3 | txn Directory age 3096 | 0 | 0 | 0 | 0 | 3096 |
| 18:23:37 | 300 | 1.4 | 3.0 | 15.0 | txn BVN1 age 3122 | 0 | 0 | 0 | 0 | 3156 |
| 18:24:37 | 300 | 1.2 | 1.7 | 31.3 | txn Directory age 3217 | 0 | 0 | 0 | 0 | 3217 |
| 18:25:39 | 300 | 0.9 | 6.5 | 101.3 | txn BVN2 age 3246 | 0 | 55 | 0 | 0 | 3276 |
| 18:26:38 | 300 | 1.0 | 2.5 | 101.0 | txn BVN3 age 3250 | 0 | 37 | 0 | 0 | 3336 |
| 18:27:39 | 300 | 1.4 | 5.3 | 51.8 | txn BVN1 age 3360 | 0 | 21 | 0 | 0 | 3397 |
| 18:28:42 | 300 | 1.6 | 43.8 | 147.9 | txn BVN2 age 3365 | 0 | 4 | 0 | 0 | 3457 |
| 18:29:42 | 300 | 1.5 | 50.4 | 100.7 | chain BVN3 age 3489 | 0 | 0 | 0 | 0 | 3517 |
| 18:30:43 | 300 | 1.5 | 58.7 | 118.2 | txn BVN2 age 3544 | 0 | 0 | 0 | 0 | 3577 |
| 18:31:43 | 300 | 1.7 | 51.1 | 91.7 | chain BVN2 age 3603 | 0 | 0 | 0 | 0 | 3636 |
