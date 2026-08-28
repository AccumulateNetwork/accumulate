# DagBFT under load: what is failing, the code behind it, and the evidence we still lack

**Status:** current as of 2026-08-20, branch `issue-4105-collection-proof-delivery`
(`a1f3584fc`). Primary evidence: soak runs `20260819T215957Z` (20 min, seized)
and `20260819T234054Z` (12h target, failed at 4.6h), both in
`test/docker/soak/runs/` with manifests. Raw 2.4 GB node-log capture for the
second run is local-only on the soak box (GitLab blob limit).

This document does four things: (1) states what is failing, with the verified
timeline; (2) records the full code review — every defect found, with file and
line; (3) lists the logging and instrumentation we were missing, because parts
of this diagnosis rest on a single surviving log window; (4) inventories every
open DagBFT issue and maps symptom → issue.

---

## 1. What is failing — the short version

**Under 10 TPS of user load at the configured 3-second block interval, primary
cross-partition delivery under-delivers almost immediately, and the recovery
machinery's response to that failure destroys the network.** Chaos
(pause/restart of one node at a time) deepens the hole but does not cause it —
every major failure ignited before or independent of the first chaos event.

The verified timeline of run `20260819T234054Z` (start 23:40:54 UTC):

| clock | minute | event | source |
|---|---|---|---|
| 23:47 | 7 | heals: 8. Normal. | monitor.csv |
| 23:52 | 12 | **heals: 3,905** — recovery already carrying thousands of messages | monitor.csv |
| 23:56 | 16 | seizewatch SEIZED: anchor BVN2→Directory `delivered=8`, undeliv climbing | seizewatch.out |
| 00:02 | 22 | **fleet CPU 123% → 2,099%** (sum over 13 containers; pinned ~2,200% for the rest of the run) | monitor.csv |
| 00:03 | 23 | *first chaos action* (pause bvn1-val1 137s) | chaos.log |
| 00:16 | 36 | loadgen begins losing `submit:`/`query:` routing — steady ~12 failures/min from here on | soak.log |
| 00:18–00:43 | 38–63 | **DN frozen at height 5,241 for ~25 min**, all healthchecks green | monitor.csv |
| 00:24–00:30 | 44–50 | loadgen rejections 74 → 1,483; throughput collapse begins | soak.log |
| 00:54–01:42 | 74–122 | **loadgen fully frozen** — `sent` pinned at 19,114 for ~48 min | soak.log |
| ≤02:47 | ≤187 | libp2p resource-manager exhaustion in full force (onset not datable — see §3) | node-logs |
| 03:06–04:12 | 206–272 | **DN frozen at 31,321 for ~66 min**, healthchecks green throughout | monitor.csv |
| 04:25 | 285 | run killed; verdict FAILED; 6 stalled synthetic channels + the anchor stall | manifest |

Two prior data points frame this:

- Run `20260819T205323Z` (commit `67b287682`, **100 ms blocks**, same 10 TPS,
  same chaos): no seizure, but **13,117 heals in 75 minutes** — recovery was
  already doing the delivery work; the #4098 commit message records one stream
  1,047 anchors deep. Primary delivery was already unreliable; fast blocks and
  heal volume papered over it.
- Run `20260819T215957Z` (adds only `f7dac217f` = 3 s blocks): seized in 16
  minutes with **zero heals ever** under aggressive 25–45 s chaos cadence.

So the block-interval fix (#4098) did not create the delivery defect; it
removed the camouflage and changed which failure mode wins.

---

## 2. The failure chain, mechanism by mechanism

Each subsection: what happens, the code, the evidence, the fix.

### 2.1 Primary delivery is fire-and-forget, and it under-delivers from the start

Every block, every validator constructs its anchor envelope and queues it;
`Dispatcher.Send` runs at the next block and **errors are logged and
discarded** — dispatch never retries
(`internal/core/crosschain/conductor.go:220-231`; the design comment in
`anchoring.go` states it: "dispatch itself never retries").

Evidence that ordinary delivery fails at scale: BVN2 produced an anchor per
3 s block for 4.6 h; the Directory's anchor ledger showed `received=8` at
minute 16 and only 172 at teardown (BVN1: 12, BVN3: 4). The *why* of the very
first lost anchors is *not yet proven* — the earliest node logs were rotated
away (§3.1) — but the surviving windows show the submission path rejecting en
masse under backpressure (§2.2), and the loadgen's independent record shows
rejections from minute 44. **This is #4111's open question and the root
investigation.**

Fix direction: a delivery path with an acknowledgment loop — track per-stream
`produced` vs destination `delivered` at the source and re-drive
unacknowledged anchors as a matter of course (#4105's thesis: collection
proofs as the delivery primitive, not only recovery). A fire-and-forget
primary plus a storm-prone healer is the worst of both.

### 2.2 The DAG-BFT submitter sheds load with no discrimination — anchors die with user traffic

`pkg/consensus/worker/worker.go:43` — `ErrBackpressure = "worker backpressure:
pending transactions exceed limit"`. When the pending queue is full, **all**
submissions are rejected equally: a lite-token send and the anchor that three
partitions need for consensus proofs are the same to it.

Evidence: 213,000+ `Failed to dispatch … worker backpressure` on BVN2's four
validators in their surviving log windows (the co-located Directory pipeline
was saturated); 43,522 on bvn2-val1 in a 23-minute window alone.

Fixes: (a) priority classes — system messages (anchors, synthetics) must not
queue behind user transactions, or must have a reserved budget; (b) the
dispatcher must treat backpressure as retryable (it is transient by
definition) instead of dropping the envelope; (c) queue-depth gauge + a WARN
at high-water (§4).

### 2.3 The healers are correct individually and a storm collectively

- `requestMissingSynthetics` runs **every block** (3 s), spawned via
  `runTask` with no overlap guard (`conductor.go:265-276`). Per stream it is
  well-behaved (jittered claim, range-proof fast path, per-scan cap —
  `synthetic.go:113+`), but each pull is 1 sequencer dial + 1 query + 1
  submit, ×3 retry attempts 250 ms apart (`synthetic.go:322-333`), on **every
  validator of every partition** with a gap.
- `healAnchors` (source-side push) re-walks the **entire** undelivered set
  every 10 s scan — one `didSign` query per missing anchor
  (`anchoring.go:86-135`) — bounded only by the 30 s scan deadline. With
  `delivered=8` and thousands produced, each scan is hundreds of RPCs.

Evidence: heals 8 → 3,905 in five minutes; fleet CPU ×17 by minute 22;
stream-open rate measured at ~33/s on a single node (stream id 358,494 by
02:47); the dial-failure storm names `ServiceType:f001` =
`private.ServiceTypeSequencer` (`internal/api/private/api.go:19`) — the
healers' target. 240,023 node-side `no live peers` failures, 101k of them for
`query:bvn2`, almost none logged *by* BVN2's own nodes (57): the load is on
the callers.

Fixes: a global heal budget (concurrent pulls per remote partition, per node),
per-stream circuit breaker (back off the whole stream after N consecutive
failures, don't retry each message 3×), and an overlap guard so scans cannot
stack.

### 2.4 One libp2p host, no resource manager configuration, and consensus lives on it

- The API node's host is created with **no resource-manager or
  connection-manager options** (`pkg/api/v3/p2p/p2p.go:100-137`; grep for
  `rcmgr`/`ResourceManager` across `pkg/api/v3/p2p`, `pkg/consensus/p2p`,
  `cmd/accumulated/run` finds nothing) — pure libp2p defaults.
- DAG-BFT consensus **shares that host and its gossipsub router**
  (`cmd/accumulated/run/dagbft.go:373`: `svcConfig.Host = inst.p2p.Host()`).
  A standalone consensus host with connection-manager watermarks exists
  (`pkg/consensus/p2p/host.go`) and is **unused** by the production wiring.
- Under the heal storm, the host's **transient outbound stream scope**
  saturates: every open then fails with `cannot reserve outbound stream:
  resource limit exceeded` — **1,358,047** occurrences in the surviving
  windows, ~235/s sustained. A paused container makes it worse (SIGSTOP'd
  peers complete TCP handshakes in-kernel but never answer negotiation, so
  each dial holds a transient slot for the full 10 s `NewStream` timeout —
  `p2p.go:273-276`), but the storm alone is sufficient.
- Consensus streams starve with everything else. This is the mechanism behind
  both DN freezes (25 min and 66 min) with healthchecks green — and behind
  why cp-4087's CometBFT-based soak survived 48 restarts: CometBFT's TCP
  stack is not on this host.

Fixes: configure the resource manager with validator-sized limits and
metrics; move consensus to the dedicated host (it exists); until then treat
API-side storms as consensus-critical incidents.

### 2.5 One missing error case turns local exhaustion into global peer poisoning

`classifyDialError` (`pkg/api/v3/p2p/dial/dialer.go:376-436`) has **no case
for resource-limit errors**. They fall through to "Unknown error, mark peer
bad" (`dialer.go:433-435`) — all 1.36M log lines are that fallback firing. So
when the *local* node runs out of stream budget, it marks every *remote* peer
known-bad, for every service. The tracker — the primary dial path since the
tracker-first optimization (`dialer.go:154-172`) — is then empty, every dial
falls back to DHT `FindPeers`, which needs streams on the same exhausted
scope, fails, and returns nothing: `no live peers` for everything, forever,
while the retry sources keep the scope pinned.

Fix: resource-limit errors → `severityDontCare` (a local condition says
nothing about the peer). This single change breaks the latch.

### 2.6 Background peer recovery decays to never

`tryDial`'s attempt ledger (`dialer.go:286-336`) is keyed per peer, the
counter increments on every attempt, and **nothing resets it on success**;
`backoffTime` doubles per attempt to a 24 h cap. A peer that failed a handful
of times during one pause is background-probed roughly once a day thereafter.
Present on both lineages; on cp-4087 it is masked because discovery-first
dialing re-finds peers anyway.

Fix: reset the counter on successful dial; cap the backoff at minutes, not a
day.

### 2.7 The same stack, client-side

The loadgen's embedded client uses the same dialer. Its complete (unrotated)
record shows steady `no live peers` from 00:16 — thirteen minutes after the
first chaos disruption — at ~12/min for the whole run: tracker rot plus the
1 s discovery timeout (`dialer.go:177-182`), no recovery. Its 95,238
"rejected" are almost all these client-side routing failures, not chain
rejections (a reporting distinction #4112/#4113 also cover).

### 2.8 Health means nothing during all of this (#4108)

Both DN freezes and the entire collapse ran under `13/13 healthy` container
status. The healthcheck does not look at ledger advance. Known, filed, worth
restating because it is the first thing everyone checks.

---

## 3. Evidence gaps — and the logging that would have closed them

### 3.1 What we lost

- **Bounded docker logging rotated away the first ~3 hours** on every busy
  node: surviving windows start 02:47 (bvn1-val4) to 04:02 (bvn2-val1). The
  onset of resource exhaustion and the first lost anchors — the two most
  important moments — are undatable from node logs. The `soak.sh` capture
  (`docker compose logs` once at teardown) inherits whatever rotation left.
- The dispatch-failure flood (88k identical ERROR lines on one node) is
  itself what pushed rotation over the early evidence. Unthrottled repeated
  error logging destroys the evidence of its own cause.
- No goroutine/RSS/stream trend exists for DI nodes at all (#4110 — no
  `/metrics` listener), so the storm's growth curve is reconstructed from a
  CSV CPU column and one stream id in an error message.

### 3.2 Logging and instrumentation to add (each maps to a blind spot above)

**Capture-side (soak harness):**
1. Stream `docker compose logs -f` into `runs/<id>/` from run start —
   rotation-proof capture (fixes §3.1; belongs with #4113).
2. Record `docker stats` per-container (not just a fleet sum) each monitor
   tick.

**Node-side counters/gauges (extends #4095/#4110; namespace `accumulate_`):**
3. `dial_total{service, outcome}` — outcome ∈ {ok, timeout, no_addrs,
   resource_limited, refused, other}. The storm and its onset would be one
   PromQL query. Log a WARN **once per state change**, not per failure.
4. `rcmgr_scope_usage{scope, resource}` / `rcmgr_scope_limit{...}` — transient
   and system scopes at minimum, plus a WARN at 80% saturation. Saturation was
   invisible until total failure.
5. `dispatch_total{destination, outcome}` + `dispatch_backpressure_total` —
   and rate-limit the ERROR line (first occurrence + count-per-minute).
6. `worker_pending{partition}` gauge + the limit as a constant label — the
   backpressure trigger is currently invisible until it fires.
7. `heal_pulls_inflight{remote}`, `heal_scan_missing{remote}` (gauge set each
   scan: how far behind), `heal_pull_total{remote, outcome}` — the heal storm
   would read as a number, not a CPU mystery. The scan-summary log line should
   carry `missing=N attempted=M`.
8. `tracker_known{service, status}` — "known-good for query:bvn2 = 0 on 8
   nodes" is THE alarm for this whole class; today the tracker's state is
   unobservable.
9. `libp2p_streams{direction}`, `libp2p_conns` per host — the 33/s open rate
   should be a graph, not an inference from a stream id.
10. Consensus: round/commit progression per partition at INFO (rate-limited),
    plus `consensus_round` / `consensus_commit_height` gauges — a frozen DN
    would name itself (#4103, #4099).
11. Peer connect/disconnect events with reason, INFO, rate-limited — pause
    /unpause chaos would be legible in the victim's own log.

**Client-side (loadgen):**
12. Split "rejected" into `submit_http_error`, `submit_routing_error`
    (`no live peers`), `submit_chain_rejection{code}` — 95k routing failures
    reported as "rejections" cost an hour of misreading (#4112).

---

## 4. DagBFT issue inventory — state and symptom map

### The active failure cluster (this document's subject)

| issue | state | symptom | mechanism (§ above) |
|---|---|---|---|
| #4111 | open, root | anchors/synthetics under-deliver at 3s blocks from minute ~12; heals 13k/75min at 100ms was the same defect masked | §2.1 — cause of the *first* lost messages unproven (evidence rotated) |
| #4115 | open | `no live peers` ×240k node-side; discovery never re-converges | §2.3–2.6 chain; timeline correction: storm precedes chaos |
| #4103 | open | DN halts under load, all containers healthy; 25min and 66min freezes this run | §2.4 consensus starved on shared host; self-recovers, mechanism of recovery unknown |
| #4098 | fix landed (`f7dac217f`) | block interval finally honored; removed the fast-block camouflage | context for #4111 |
| #4108 | open | healthchecks green through every failure above | §2.8 |
| #4086 | open | fixed 60-block reconcile grace wrong whenever anchoring lags | same block-vs-time class as the #4111 hypothesis space |
| #4073 | open (main too) | lost prefix/tail undetectable by gap-based healing | why `streams-final` showed `dn→bvn* produced=1 received=0` stalls |

### Harness / observability (feeds the diagnosis quality)

| issue | state | summary |
|---|---|---|
| #4110 | open | DI nodes serve no `/metrics` listener at all — node panels structurally 0 |
| #4112 | open | loadgen: grow timeouts vanish from accounting; identities=0 with clean exit |
| #4113 | open | harness declares seized runs clean; streams.py fabricates health from failed queries, ignores anchor ledgers |
| #4114 | open | soakmon: heals cards wired to dead source; query failures render as zero traffic; diagonal hidden |
| #4093/#4094 | open/closed | absent metrics rendered as 0 (monitor-side rule) |
| #4095 | open | six of eight metric families never implemented node-side |
| #4075 | open | consensus-status heal fields dropped in consolidation (partially restored on DI, `12cd7a723`) |
| #4100/#4099 | open | soak harness with provenance / state-derived block-rate measurement |
| #4102/#4104/#4107/#4109 | open | parallel-loadtest defects; retired as a measurement tool |

### Foundation issues (pre-existing, still open, adjacent)

| issue | state | summary |
|---|---|---|
| #4105 | open, umbrella | collection proofs as the delivery primitive — the structural answer to §2.1 |
| #4056/#4055 | open | anchor healing / steady-state emission via collection proofs |
| #4057 | open | consensus does not resume after node outage |
| #4059 | open | DN→BVN network-definition updates never reach BVNs (missing-prefix class; blocks on-chain promotion, P3) |
| #4060 | open | discovery bootstrap bound to 127.0.0.1 in multi-host deployments |
| #4054 | open (largely addressed in code comments: gossipsub/DHT init order) | rounds stuck at 0 after genesis |
| #4101 | open | Dockerfile still installs cometbft on dagbft-integration |
| #4097 | open | halt unwired, anchor recovery unowned, consensus-testnet failing |
| #4071 | open | routing-table bit-width imbalance (v1.5 target) |

### Closed recently (context for what already got fixed)

#4085 (API OOM / kad-dht stream growth), #4089 (discovery goroutine leak —
`forwardUntil`), #4090 (one collection proof per block), #4087 (collection
proofs validated on cp lineage, 22.6h soak PASS), #4091 (unroutable
find-service addresses), #4094 (monitor absent-metric rule).

---

## 5. Recommended order of work

1. **Latch-breakers (small, high leverage):** classify resource-limit errors
   as don't-care (§2.5); reset `tryDial` backoff on success (§2.6).
2. **Budgets:** configure the libp2p resource manager + export scope metrics
   (§2.4, §3.2-4); heal budget + per-stream circuit breaker (§2.3); dispatcher
   retry-on-backpressure + system-message priority (§2.2).
3. **Isolation:** consensus onto its own host (`pkg/consensus/p2p` exists,
   unused) (§2.4).
4. **Evidence:** rotation-proof log capture + the §3.2 counters, so the next
   run diagnoses itself.
5. **Then re-run 12 h chaos-free at 3 s blocks** to isolate the primary
   delivery defect (#4111) with working transport — followed by the chaos run.

A 12-hour chaos soak is not currently survivable, and item 5 is not
interpretable until items 1–2 land. The pre-chaos delivery stall (#4111) is
the root; everything else determines whether the network degrades gracefully
or eats itself when delivery falters.
