# Synthetic-healing chaos soak — results

Validation of DAG-BFT synthetic/anchor healing (#4064, #4067, #4070) under
continuous membership chaos and cross-partition load, on a real 12-container
libp2p network (3 BVNs × 4 validators, dual DN+BVN, executor v2 line).

Harness: `test/docker/synth-heal/soak/` (soak.sh, soakmon.py, network.yml,
docker-compose.yml) + `tools/cmd/loadgen`. Machine: 24-core laptop, single NVMe.

## Headline results

| What | Result |
|------|--------|
| **#4070 synthetic-wedge fix (Jiuquan)** | **Confirmed.** 20h, `stuck=1`, `errors=0`, no permanent wedge through 79 chaos events. On plain v2 the same soak wedged permanently in ~15 min. |
| **Anchor healing** | 0 wedges; chaos-lost anchors heal, no flood (source-push is safe while anchor *drops* are off). |
| **Loadgen endpoint rotation** | Real rejections fell to ~0.003% (4 / 135k); the rest are intentional `fail:*`. |
| **Node OOM resilience** | 4g + `restart: unless-stopped` — memory flat (0.5–0.8 GiB / 4 GiB), no OOM deaths, chaos restarts self-heal. |

## Run history

- **Run 1 — v2, anchor drops + source-push anchor healing ON.** Catastrophic
  flood: every validator re-pushed the undelivered anchor tail. 1,997% CPU,
  mempool pinned at cap (decoded 100% block anchors), CPU ~10× a healthy run.
  Root cause: source-push `healAnchors` ignited by a deliberate *prefix* drop
  (seq 0/1/2) that healing cannot detect. Not the healer we intend to ship.
- **Run 2 — anchor healing OFF, anchor drops ON.** BVN1/BVN3 froze at height 5:
  removing anchor retry while still dropping anchors leaves the destination's
  delivered-sequence wedged forever. Confirmed anchors have no self-heal without
  a retry mechanism.
- **Run 3 — anchor drops OFF, synthetic drops ON, executor `v2`.** Synthetic
  stream `BVN2→BVN3` wedged **permanently** in ~15 min: `delivered` frozen,
  `stuck` climbing to 98, gap growing to 650+. **Root cause (this is the finding
  the whole effort turned on): pre-Jiuquan, a synthetic whose covering DN anchor
  arrives late is recorded a TERMINAL FAILURE instead of held; the healer's
  byte-identical re-submission is then deduplicated and never re-processed
  (#4070). The proof is not "permanently invalid" — the anchor must arrive; the
  bug is throwing the message away a few blocks too early.**
- **Run 4 — executor `v2-jiuquan`.** The #4070 fix (hold-pending, re-attempt in
  place when the anchor lands) turned the wedge into a transient: 11h+ with
  `stuck=1`, no growing gap. Also surfaced a node OOM: the single API-serving
  node hit the old 2 GiB limit after ~6h and, with no restart policy, stayed
  dead — taking observability with it.
- **Run 5 — Jiuquan + all harness/infra fixes.** 20h, `stuck=1`, `errors=0`,
  ~16.7k synthetic heals recovering 1,397 injected drops + chaos losses, through
  79 chaos events (39 restart, 40 pause). 135k txns @ ~1.9 TPS, grew to 7,063
  accounts / 442 ADIs / 826 token issuers. Real rejections ~4.

## Fixes made during the effort

**Protocol / config**
- Run at **`v2-jiuquan`** (was `v2`) — activates the #4070 held-and-retry.
  Wedge eliminated. (Kourou goes further with receiver-side collection proofs;
  see "Not covered".)

**Infra (`docker-compose.yml`)**
- All 12 nodes given **4 GiB** (was 2) and **`restart: unless-stopped`** — OOMs
  self-heal instead of killing the network + observability.
- All 12 nodes **expose host API ports 26660–26671** (was only s1a) so the
  host-side loadgen can reach every node.

**Loadgen (`tools/cmd/loadgen`)**
- **Endpoint rotation, pinned by signer** — submissions/queries spread across all
  12 nodes, but a signer's (ordered) transactions always go to one node so the
  executor's monotonic-timestamp rule is preserved. Killed the connection-error
  rejections that came from talking only to the one chaos-disrupted node.
  (First attempt round-robined per-transaction and broke same-signer ordering,
  which failed the bootstrap; pin-by-signer is the fix.)
- **100-sub-treasury bootstrap** — seed a spread base of funded sources before
  the workload, so load originates from every BVN, not just the treasury's.
- **Per-account accounting** (observe-only) — exact token dead-reckoning +
  `ComputeTransactionFee` credit dead-reckoning + periodic reconcile against
  chain; surfaces refund drift.
- **Refund/failure actions** — 6 `fail:*` transactions (overspend, overburn
  tokens/credits, send/data-to-void, sub-adi-on-void) to exercise the refund
  path and the reconciler.

## Open items

- **BVN2 source concentration (~55–68%).** The 100-sub-treasury fan-out fixed the
  `send-tokens-lite` path but `add-credits-lite`, `write-data-lite`, `burn-tokens`
  and all `growAsync` ADI funding still source from the treasury (on BVN2).
  Fix: route those actions + ADI funding through random sub-treasuries. Not a
  network defect — a load-shape artifact.
- **`fail:overburn-credits` rejects at validation, not a refund** — it fails at
  submit (insufficient credits to burn), so no fee is charged to refund. The
  other 5 `fail:*` do exercise the refund path.
- **Accounting credit drift metric** is dominated by the treasury's huge,
  under-modeled balance; normal-account refund drift is what matters and the
  reconciler handles it. Cosmetic.

## Not covered by these runs

- **Kourou receiver-side collection-proof anchor healing (#4048/#4056).** These
  runs use the older source-push anchor healer (safe here only because anchor
  drops are off). The collection-proof path — the intended production anchor
  healing — lives on `dagbft-integration` and was not exercised here.
- **Mixed-version interop** (new binary alongside the currently-deployed version)
  — the next test.
