# DAG-BFT week-long churn soak

Prove that a DAG-BFT network survives a week of continuous membership churn:
followers born from nothing catch all the way up, become validators, and
validators drop out — while tiny, wide-spread load keeps state growing so
"catch up" always means real work.

Companion docs: `fast-validator-deployment.md` (the fastsync mechanism this
exercises), `accumulated-dagbft.md`. Issues: #4058 (fast deployment), #4050
(follower→validator), #4057 (outage recovery, merged into this lineage).

## What is being proven

1. **Catch-up from nothing.** A brand-new node (fresh keys, empty database)
   can join a network that has been running for days, fully converge on both
   its partitions (DN + one BVN), and stay converged.
2. **Promotion.** That node can be added to the validator set on-chain and
   participate in consensus (its headers get certified, it signs its share of
   blocks) without disturbing the network.
3. **Demotion/loss.** Validators can be removed (gracefully or by container
   kill) and the network keeps producing blocks and anchors.
4. **Endurance.** All of the above repeatedly, for 7 days, with state
   growing the whole time (many thousands of distinct accounts), memory flat,
   disk bounded.

## Topology and invariants

Base network: the existing `test/docker/docker-compose.yml` net (3 BVNs,
nodes named `bvn{1,2,3}-val{1..4}`, every node runs dual DN + its BVN,
host API ports 26660+). Dynamic nodes are provisioned as `churn-N`
(N monotonically increasing, never reused), each assigned to one BVN.

Hard invariants the controller must never violate:

- **Total live nodes ∈ [6, 12]** (validators + followers, all partitions).
- **Per-partition validators ∈ [1, 5]** — this includes the DN committee
  (every node is dual, so DN committee size = total validators).
- **One membership change at a time.** Wait for the committee epoch to
  settle on all live nodes before the next add/remove.
- **Never remove a validator while its partition has ≤1**, and never remove
  one within 10 minutes of the last committee change.
- **Prefer dropping old validators over young ones** so every original node
  is eventually replaced by a churn-born one (full generational turnover is
  the strongest possible pass).

## Prerequisite gaps (close these first, in order)

Each has an acceptance test; do not start the week run until all pass.

**P1 — BVN fastsync (phase 3b).** `accumulated fastsync` takes
`--partition` generically but only the Directory path is proven;
`test/test-4058-rejoin.sh` explicitly leaves the victim's BVN wedged. A
brand-new node needs BOTH partitions. Per `fast-validator-deployment.md`,
BVN minor-root verification rides the DN spine (`PartitionAnchorReceipt`).
*Accept:* extend the rejoin test (or a copy) so the victim fastsyncs DN and
its BVN, and BOTH converge to the live tip. This is protocol-adjacent work —
if it turns out phase 3b is unimplemented (not just untested), flag Paul
before building it on a reduced model.

**P2 — Follower mode.** A node whose key is NOT in the committee must still
follow: receive certificates via gossip, execute blocks, serve API queries,
and track committee epochs. Verify the dagbft run path tolerates a
non-committee validator key (it should idle in consensus but execute).
*Accept:* start a 13th node with fresh keys against a running net (config
via the same init tooling, `run.Config` validator-key per #4050's
f3120b9b2), fastsync both partitions, and watch its ledger heights track the
network for 30+ minutes.

**P3 — On-chain add/remove validator.** Promotion = updating the
NetworkDefinition (`protocol/network_def.go` AddValidator/RemoveValidator)
on `dn.acme/network`, signed by the DN operators book (test-net operator
keys are in the compose volumes / init output). The executor emits
`ValidatorUpdate`s → `ExecutorBridge.handleValidatorUpdates` →
`Service.onValidatorSetChange` → `UpdateCommittee` (epoch+1) on every node
deterministically. *Accept:* on a running net, add the P2 follower's key;
every node logs "Validator set changed, updating committee" at the same
block height; the promoted node's headers start getting certified (its
author key appears in "Created certificate" signers on peers); then remove
one original validator and confirm blocks keep flowing. Watch for the epoch
race: the joiner must land in the same epoch numbering as the rest —
epoch is bumped per change, so a node that missed changes has a stale epoch
and its headers get rejected ("Header epoch mismatch"). If that happens the
committee-epoch model needs to be derived from state, not a local counter —
protocol work, flag Paul.

## Load generator — `test/cmd/churn-load`

Tiny rate, maximum account spread. One process on the host, journal on disk
so it survives restarts and so verification can sample known accounts.

- Faucet key: `ed25519.NewKeyFromSeed(storage.Key{}.Append("FAUCET"))`
  (established recipe), submit via v3 JSON-RPC to a rotating list of node
  endpoints (skip dead ones).
- Default rate **1 tx / 3 s** (~0.33 tps — "a tiny bit of load"; ~200k txs
  over the week). Flags: `-i` interval, `-s` endpoints, `-journal` path.
- Mix per tick: **80%** create + fund a brand-new lite account (random key —
  routing hashes spread these across all BVNs), **15%** transfer between two
  random previously-created accounts, **5%** touch an account from the
  oldest 10% (keeps ancient state hot so fastsync must carry it).
- Journal: append-only JSONL `{time, kind, account, txid}` under
  `/tmp/dagbft-churn/load-journal.jsonl`.
- On any submit error: log and continue (never crash, never retry-storm).

## Churn controller — `test/churn/controller.sh`

A single long-running bash loop (nohup + setsid, survives session exits),
state in `/tmp/dagbft-churn/state.json`, every action appended to
`/tmp/dagbft-churn/journal.jsonl`, all output to bounded logs. Dry-run flag
for shakedown. Cadence: one event every **2–6 h** (random), compressed to
**20–30 min** for the shakedown day.

Event loop (pick by weighted rules, respecting invariants):

1. **JOIN** (when total < 12): provision `churn-N` — fresh keys + config
   from init template, assign to the BVN with the fewest nodes, fastsync DN
   + that BVN from a live peer, write rejoin seed, start container, then
   **verify convergence**: both ledger heights within 50 of the network tip
   and still tracking 10 min later, and a sample of 20 random journal
   accounts return identical balances from the new node and an old node.
2. **PROMOTE** (when a converged follower exists and its partition has <5
   validators): submit the NetworkDefinition update, wait for "Validator
   set changed" on all nodes, then verify participation: within 15 min the
   new key appears as a certificate author/signer on peers and the node
   itself logs Created certificate.
3. **DROP** (when total > 6 and partition > 1 validator, and last change
   >10 min ago): 70% graceful (on-chain remove, wait for epoch settle, stop
   container, delete volume), 30% rude (kill container first, then on-chain
   remove after 5 min — exercises #4057 loss recovery). Prefer oldest.

Health monitor (same process, every 5 min, independent of events):

- Every live node: DN height and BVN height advancing since last check.
- Anchors: `received == delivered` (within a small lag) on every partition.
- Disk < 45% → at 45% run `docker builder prune -f` + prune dropped
  volumes; at 50% **freeze** (stop load + events, keep network up, alert).
- Container restarts/OOMs (nodes need 2g — ad36f9a68).
- On any failed check: **freeze churn** (never tear down), capture
  `docker logs --since 30m` from every node into
  `/tmp/dagbft-churn/incident-<ts>/`, and write `FROZEN: <reason>` to the
  status file. A frozen soak is a finding, not a failure of the harness.

`test/churn/status.sh` prints: uptime, node roster (name, BVN, role, age),
per-partition validator count and heights, last 5 events with verdicts,
frozen/active, disk, journal tx count. This is what the monitoring model
runs each check-in.

## Verdicts

- **Per-JOIN:** convergence check passed (heights + sampled balances).
- **Per-PROMOTE:** committee updated on all nodes at one height; new
  validator certifying within 15 min.
- **Per-DROP:** all partitions produce blocks and anchors continuously
  through the removal (no gap > 2 min).
- **Week PASS:** ≥15 successful JOIN→PROMOTE cycles and ≥15 drops; ≥1 full
  generational turnover on some BVN; zero frozen incidents (or every freeze
  root-caused and fixed, then the clock restarted); memory flat; disk <50%
  throughout.

## Operating notes for the monitoring model

- Check in a few times a day: run `status.sh`, read the last journal lines.
  If FROZEN: read the incident bundle, summarize the failure signature for
  Paul, do NOT restart or tear down anything.
- Never touch containers not named `acc-*`/`churn-*`; never run
  `devnet reset`; keep disk under 50%; redirect all command output to files.
- The known failure signatures and their fixes live in
  `~/.claude/.../memory/project_dagbft_network_status.md` — check a wedge
  against those before calling it new (silent stall = look for "Created
  header payload=N" never received by peers; round wedge = check
  per-round cert counts; mesh = "Gossip mesh status" lines).

## Build order

1. P1 → P2 → P3 (each with its acceptance test green).
2. `churn-load` + 1-hour smoke (accounts spread across all 3 BVNs).
3. Controller + status in dry-run against a running net.
4. **Shakedown: 24 h** at compressed cadence (~50 events). Fix what breaks.
5. **Week run** at 2–6 h cadence. Tag the git SHA the images were built
   from in the state file; do not rebuild mid-run.
