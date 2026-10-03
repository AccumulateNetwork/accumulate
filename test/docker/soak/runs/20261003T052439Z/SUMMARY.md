# Run 20261003T052439Z — follower bootstrap test

Paul, 2026-10-03: "Launch the network, wait 5 minutes, test follower
bootstraps. After all 3 followers are stable and caught up, wait 5 minutes,
do another 3, repeat, then close everything and write it up."

## Provenance

| | |
|---|---|
| network | lead `issue-4205-lead` @ `78e1463a0` (built by soak.sh at 05:24Z): #4502 #4503 #4505 #4506 #4507 #4508 #4510 and the harness changes of 2026-10-02/03 |
| driver | `test/docker/soak/bootstrap-test.py` @ `97b625a84` (the cycles); first attempt @ `78e1463a0` |
| network shape | 3 BVNs × 4 validators + Directory (12), comparison follower bvn3-fol1, late followers bvn1-fol1, bvn2-fol1, bvn3-fol2; BlockchainDB; 1 s blocks; mem_limit 2048 MiB, GOMEMLIMIT 1300 MiB |
| load | 1 tx/s, full mix, chaos off (`72h-watch.conf`); 3,214 sent, **0 rejected**, 2 skipped |
| started / stopped | 05:24:39Z / 06:22:05Z |

## Result: PASSED — 18 of 18 partition joins

| cycle | joins synced | ACTIVE after | synced after | behind at the dwell | drops out of agreement in 5 min |
|---|---|---|---|---|---|
| 1 (05:57:05Z) | 6/6 | 50–60 s | 171–192 s | 0 on all six | 0 |
| 2 (06:05:17Z) | 6/6 | 50–60 s | 181–192 s | 0 | 0 |
| 3 (06:13:31Z) | 6/6 | 50–81 s | 172–212 s | 0 | 0 |

"Synced" is the specification's: 20 perfect blocks in the overlap, then 100
blocks held with no regression (`The node is synced`). The per-join table is
`bootstrap-test.md`. Throughout, every partition was live, every node ACTIVE
and 0 behind, and the worst RSS was ~600 MiB of the 2 GiB limit (memAlarm
off).

## The first attempt did not test the join (driver defect, fixed)

The first cycle (05:33:52Z) re-added the followers with their stores cleared
but **without** `join-running-network`: the driver's config edit carried a sed
pattern whose quotes the nested shells stripped, so it matched nothing. The
followers started as "the first node of a network" and replayed from genesis
under the #4506 watch — and reached agreement that way (`This node is
executing in agreement; its own root matched its signed anchor`, block
394/415) — but that is the genesis path, not a bootstrap. The driver waited
for join lines that path never writes; it was stopped at 05:55Z, fixed
(`e929b744f`: a pattern that survives the shells, a check that every config
carries the flag, and a cycle fails if a follower replays from genesis), and
the three cycles were run on the same network with `--attach` (`97b625a84`).
The record of both attempts is in `chaos.log`.

## What this run does and does not prove

Proves, on the code now on the lead: a follower joining a running network
under load reaches ACTIVE in about a minute and synced in about three, nine
times out of nine per partition kind, with no drops. Does not test the
restart cases (graceful, kill, torn store, revival) — those are the next run
(PLAN "Restart, torn stores and quorum loss").

Deleted after the write-up: `node-logs-live.txt` (292 MB), `node-logs.txt` (241 MB).
