# Run 20260929T152146Z — summary

Written 2026-10-02 after the run was stopped. The bulk logs it summarises were
deleted afterwards (see "What was kept").

## What it was for

The Docker proof of #4466: a fresh 12-validator network on the lead, steady
load, then one deliberate whole-network restart, every validator recreated at
once. It then served as the test bed for follower joins and history backfill
(#4469, #4471, #4472, #4483, #4484) until Paul stopped it on 2026-10-02 as
having "proved what it is going to prove".

## Provenance

| | |
|---|---|
| validators | lead `issue-4205-lead` @ **`03945b6ebffcc71035b330b0f5c767c4c1e32c2a`** for the whole run; image `disoak-bvn1-val1` `sha256:2c0af9ae0beb566af0d0682178b31238c7822a885eb79ca1b26f34e823529bbe` |
| comparison follower | `acc-bvn3-fol1`, same image, started with the network |
| late followers | rebuilt per cycle (below); every rebuild is in the manifest's disturbance record |
| network | 3 BVNs × 4 validators + Directory (12), bootstrap; executor `v2-kourou`; BlockchainDB; 1 s blocks; mem_limit 2048 MiB, GOMEMLIMIT 1700 MiB |
| load | 1 tx/s, full mix, chaos off |
| started / stopped | 2026-09-29T15:21:46Z / 2026-10-02T05:53:45Z (62 h 32 min; target was 72 h) |

## Timeline

| when (Z) | what | follower image |
|---|---|---|
| 09-29 15:24 | load starts | — |
| 09-29 15:41:24 | **whole-network restart**: 12 validators + bvn3-fol1 recreated at once | — |
| 09-29 15:53–15:59 | first follower add (manual), Directory stuck ~4 min at its seed round → #4469 | 03945b6eb |
| 09-29 21:53 | bvn1-fol1 re-added on the #4469 fix: Directory ACTIVE in 41 s (was 5 min 38 s) | 9cc09eac0 |
| 09-29 22:50 → 23:24 | follower cycles 1–3: three followers removed and re-added together, 3 times | 9cc09eac0, then 72b1d43b6 (#4471) |
| 09-30 01:07 | cycle 4 (#4472: backfill beside the root watch) | be56c8757 |
| 09-30 21:26 | cycle 5 — consensus/ not cleared by the harness (stale checkpoint) → #4484 | be8a5cfce (CometBFT removal) |
| 10-01 21:01 | cycle 6 — three truly fresh followers (databases and consensus/ cleared) | 778f1f2d0 |
| 10-02 05:53:45 | stopped (load generator interrupted; harness recorded and tore down) | — |

## Results

**The whole-network restart (#4466).** Every validator resumed from its own
block ("too few serve to sign an anchor"), 26 resume lines in 2 s. No stream
head was blocked after the restart in 62 h (the 30 such lines in the log are
all before it, at 15:29 and 15:38). The network ran on for 62 h: Directory
height 40 → 222,577 (~1 block/s). All 12 Directory validators answered at
every sample.

**Load.** 241,398 transactions generated, 9 rejected, 14 skipped. All 9
rejections were in the first minutes, before the restart.

**Heals: 45 in 62 h, all on three occasions, none otherwise.**

| when | heals | cause |
|---|---|---|
| 09-29 15:42 | 9 | the restart: anchors in flight when memory was wiped (a genuine drop) |
| 09-29 23:13 | +20 | bvn1-val3 missing 20 Directory anchors a minute after cycle 3's followers joined (#4473) |
| 09-30 21:27 | +16 | bvn1-val4 missing 16 anchors a minute after cycle 5's followers joined (#4473) |

**Follower joins: 36 partition joins over 6 cycles, 0 drops out of agreement.**
Time to ACTIVE in seconds (Directory / BVN, per follower):

| cycle | bvn1-fol1 | bvn2-fol1 | bvn3-fol2 | note |
|---|---|---|---|---|
| 1 | 52 / 37 | 67 / 57 | 62 / 57 | consensus/ kept (harness fault, found in cycle 5) |
| 2 | 47 / 37 | 47 / 42 | 47 / 42 | consensus/ kept |
| 3 | 86 / 41 | 56 / 41 | 56 / 41 | consensus/ kept |
| 4 | 56 / 46 | 61 / 46 | 61 / 46 | consensus/ kept |
| 5 | 527 / 138 | 193 / 148 | 577 / 143 | 20 h-old checkpoint read as empty blocks (#4484) |
| 6 | 219 / 370 | 113 / 250 | 113 / 320 | truly fresh, Directory height ~191,000 |

**History backfill.** Cycles 3 and 4 (Directory height 28,000–36,000): every
follower partition logged "The backfill is done", in 21–37 min; cycle 4's
done lines carry `executed` at live height, proving the root checks kept
running (#4472). Cycle 6 (height 191,000): **no backfill finished** in 8 h —
the Directory anchor pool's signature chain is 2.3M entries, the walk
restarted from entry 0 on every failure, and a validator's "query capacity
exhausted" ended the attempt (#4493). The join itself was unaffected (all
ACTIVE, 0 behind).

**The comparison follower** stayed in agreement for 62 h after resuming at
the restart (0 root mismatches), but no line said so; the dashboard showed
"executing, not yet proven" throughout (#4483).

## Log digest

The 16 GB node log (`node-logs-live.txt`) was reduced to `log-digest.txt`
(lines by level, the top 40 warnings and errors folded, the first 200 key
events, raw keys stripped). Headline: 60.6 M INFO, 116 WARN, 5,923 ERROR.
Notable:
- 5,870 × ERROR `Local delivery failed principal: acc://void-….acme/tokens not found` — **expected**: the load generator's deliberate `fail:send-to-void` refund producer (tools/cmd/loadgen/failures.go:36). That the refunds landed was not checked here.
- 39 × WARN `Permanent layer refused a write: this shape is not write-once`.
- Every node logs its private keys at Info on start (`Loading RawPrivateKey address=`; #4494). The digest strips them.

## Issues this run produced or proved

Proved on Docker: #4466 (whole restart), #4469 (catch-up, first cause),
#4471/#4472 (backfill visible and beside the root watch).
Found: #4469's remaining cause (response-queue saturation, fixed on the lead
since), #4473 (anchors dropped at follower joins), #4483 (resumed node never
says it agrees, fixed), #4484 (wiped store + stale checkpoint, fixed),
#4486 (harness does not capture re-added followers' logs), #4493 (backfill
restarts from entry 0, fixed on the lead since), #4482 (backfill page size).

Fixed on the lead since this run's images and **not yet on Docker**: #4485
sectioned BPT sync, redirection (#4485 E24–E26, #4487), #4493, #4495 overlap,
#4496/#4497, #4470 + the pre-Verify check, #4489/#4490.

## What was kept

Committed: this summary, `manifest.md` (with the disturbance record),
`follower-cycles.md`, `follower-cycles.sh`, `network-definition.json`,
`run.json`, `config/`, `log-digest.txt`.
On disk, not committed (over GitLab's 1 MiB blob limit or derived):
`monitor.csv`, `stats.csv`, `mem.csv`, `submissions.csv`, `follower.csv`,
`storage-stats.csv`, `readprobe*.csv`, `soak.log`, `follower-report.md`.
Deleted: `node-logs-live.txt` (16 GB), 61 hourly profile/metric snapshots
(`hourly-*`, 2.2 GB), `cycle5-follower-logs/` (24 MB).
