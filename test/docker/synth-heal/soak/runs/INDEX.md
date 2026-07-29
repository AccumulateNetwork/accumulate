# Soak runs

Every run appends one row. Details in `<runId>/manifest.md`.

| run | commit | executor | healing | elapsed | exit | dn height | heals | note |
|---|---|---|---|---|---|---|---|---|
| [20260725T233615Z](20260725T233615Z/manifest.md) | `v1.4.4.2-20-ga048e0035` | v2-jiuquan | synthetic+anchor | 4.96h of 24h | STOPPED | 7→12230 | 407 syn / 495 anc | stopped early for the v1.4.5 build; no wedge in ~5h |
| [20260727T053231Z](20260727T053231Z/manifest.md) | `v1.4.5-2-gd5e5be6c2` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.09h | 0 | 9→297 | 0→3142 | #4073 proof A: 5m with the interval reconcile |
| [20260727T054232Z](20260727T054232Z/manifest.md) | `v1.4.5-2-gd5e5be6c2-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.09h | 0 | 28→244 | 0→12 | #4073 proof B: no fix, DN->BVN1 seq 1-2 dropped, major blocks every minute |
| [20260727T055020Z](20260727T055020Z/manifest.md) | `v1.4.5-2-gd5e5be6c2-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.11h | 0 | 7→273 | 0→97 | #4073 proof B2: no fix, tail of DN->BVN1 dropped |
| [20260727T055822Z](20260727T055822Z/manifest.md) | `v1.4.5-2-gd5e5be6c2-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.1h | 0 | 28→275 | 0→6669 | #4073 proof C: WITH fix, same drops as B2 |

## #4073 short-run series, 2026-07-27

Four 5-minute runs attempting a system-level reproduction of the lost-prefix
stall. **None reproduced it, and that is a property of the bug, not a gap in
effort** — see below.

| run | image | drops | major blocks | outcome |
|---|---|---|---|---|
| 20260727T053231Z | `acc-4073:test` (fix) | `*:%499+3` | 12h | clean; DN produced **nothing** to any BVN in 5 min |
| 20260727T054232Z | `acc-release:v1.4.5` (no fix) | `bvn-BVN1:%997+3` | 1 min | DN→BVN1 7/7 delivered — recovered without the fix |
| 20260727T055020Z | `acc-release:v1.4.5` (no fix) | `bvn-BVN1:%7+1` | 1 min | DN→BVN1 7/7 delivered — recovered without the fix |
| 20260727T055822Z | `acc-4073:test` (fix) | `bvn-BVN1:%7+1` | 1 min | clean, 6669 heals, no seizure — parity with the run above |

**Why a 5-minute soak cannot reproduce #4073.** The bug requires a loss with
*nothing following it for a long time*. With the stock `majorBlockSchedule` of
`0 */12 * * *` the DN→BVN stream carries ~2 messages a day — quiet enough to
break, but it produces nothing at all inside a 5-minute window (run 1). Raising
the schedule to once a minute makes the stream active enough to exercise, but
that hands the ordinary #4064 gap healer exactly the later message it needs, and
it recovers on its own (runs 2 and 3). The precondition and the short window are
mutually exclusive.

**The deterministic proof is `TestSyntheticHealingLostPrefix`**, which
constructs the condition directly: one synthetic, dropped, with no traffic behind
it. Verified red/green — with the reconcile branch disabled the produced
synthetic never delivers.

What the short runs do establish: the fix causes no regression in normal healing
(run 4 matches run 3 under identical drops), the harness now supports sub-hour
durations, and the `undeliv` metric and its seizewatch trip are wired in.
| [20260727T061434Z](20260727T061434Z/manifest.md) | `v1.4.5-3-ge39a450ff` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.1h | 0 | 8→234 | 0→0 | #4073 proof D: NO fix, sparse load 0.2tps, 50% synthetic drop into BVN1 |
| [20260727T062207Z](20260727T062207Z/manifest.md) | `v1.4.5-3-ge39a450ff-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.08h | 0 | 7→197 | 0→6 | #4073 proof E: WITH fix, identical config to D |
| [20260727T063116Z](20260727T063116Z/manifest.md) | `v1.4.5-3-ge39a450ff-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.09h | 0 | 8→249 | 0→31 | #4073 proof F: NO fix, 0.02tps so every channel is quiet |
| [20260727T063833Z](20260727T063833Z/manifest.md) | `v1.4.5-3-ge39a450ff-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.08h | 0 | 9→209 | 0→4 | #4073 proof G: NO fix, quiet channels (no bootstrap) |
| [20260727T064442Z](20260727T064442Z/manifest.md) | `v1.4.5-3-ge39a450ff-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.09h | 0 | 8→215 | 0→426 | #4073 proof H: NO fix, quiet channels, 50% drop all destinations |
| [20260727T065123Z](20260727T065123Z/manifest.md) | `v1.4.5-3-ge39a450ff-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.08h | 0 | 8→199 | 0→21 | #4073 proof I: WITH fix, identical config to H |
| [20260727T221813Z](20260727T221813Z/manifest.md) | `v1.4.5-5-geeafbfb60-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.09h | 0 | 8→207 | 0→31 | #4073 A/B rep 1: WITH fix + 1s reconcile, run H config |
| [20260727T223014Z](20260727T223014Z/manifest.md) | `v1.4.5-5-geeafbfb60-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.08h | 0 | 8→190 | 0→286 | #4073 diagnostic: is the reconcile running at all? |
| [20260727T233228Z](20260727T233228Z/manifest.md) | `v1.4.5-5-geeafbfb60-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.09h | 0 | 8→218 | 0→168 | #4073 grace: does the reconcile stop racing delivery in docker? |
| [20260728T000643Z](20260728T000643Z/manifest.md) | `v1.4.5-5-geeafbfb60-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.08h | 0 | 8→216 | 0→0 | #4073 J: NO fix, 100% drop into BVN3 |
| [20260728T001321Z](20260728T001321Z/manifest.md) | `v1.4.5-5-geeafbfb60-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.12h | 0 | 25→297 | 0→1131 | #4073 K: WITH fix, ~100% drop everywhere |
| [20260728T002325Z](20260728T002325Z/manifest.md) | `v1.4.5-5-geeafbfb60-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.09h | 1 | 8→7 | 0→0 | diag2 |
| [20260728T002848Z](20260728T002848Z/manifest.md) | `v1.4.5-5-geeafbfb60-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.09h | 0 | 7→237 | 0→874 | diag3 |
| [20260728T004607Z](20260728T004607Z/manifest.md) | `v1.4.5-5-geeafbfb60-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.09h | 0 | 8→224 | 0→1410 | #4073 N: WITH per-sequence claim — same config as K which stalled 552 |
| [20260728T010357Z](20260728T010357Z/manifest.md) | `v1.4.5-5-geeafbfb60-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.17h | 0 | 8→397 | 0→2846 | #4073 P: grace 60 blocks + per-sequence claim |
| [20260728T011520Z](20260728T011520Z/manifest.md) | `v1.4.5-5-geeafbfb60-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.1h | 0 | 13→257 | 0→2917 | #4073 Q: 5m load then 6m idle so tail losses age past the 60-block grace |
| [20260728T013006Z](20260728T013006Z/manifest.md) | `v1.4.5-5-geeafbfb60-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.08h | 0 | 8→209 | 0→524 | #4073 R: ISOLATION — gap healer OFF, reconcile is the only recovery path |

## #4073 attribution attempts, 2026-07-27/28 — the harness cannot test this

Runs J through R tried to demonstrate the interval reconcile recovering a real
loss in docker. **None could, and the reason is the injector, not the fix.**

`synthDropper.tryDrop` records each `(destination, sequence)` in a `dropped` map
and returns false on every later sighting — deliberately, so a healed
re-submission gets through. The source re-dispatches, the second attempt passes,
and the message arrives. **The injector produces a delay, not a loss.**

Proven by run R: the gap healer compiled out, so the reconcile was the only
recovery path, 355 drops injected — and `received == produced` on every channel.
With no healer at all, nothing was lost.

This invalidates the earlier "reproductions" in this file: run H's
`produced=12 received=11` and run K's 552 undelivered were in-flight messages
caught at the instant the run ended, not durable losses.

It also explains the genuine 24h case. DN→BVN1 produced two messages and went
silent; with no later traffic there was no re-dispatch, so that drop became
permanent. That is the condition #4073 addresses, and it needs a channel that
goes quiet — which continuous load prevents by construction.

**To test this in docker the injector needs a permanent-drop mode** that drops a
sequence every time rather than once. Until then the e2e test
`TestSyntheticHealingLostPrefix` is the only thing that exercises the mechanism.

What these runs did establish: the reconcile no longer misbehaves. Attempts went
from 0 (silently gated off) to firing correctly, and failures from 102/102 to
~3, with zero premature pulls.
| [20260728T161659Z](20260728T161659Z/manifest.md) | `v1.4.5-6-g988486db9-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.08h | 0 | 8→197 | 0→2974 | #4073 CONTROL: permanent drops into BVN3, reconcile OFF |
| [20260728T162643Z](20260728T162643Z/manifest.md) | `v1.4.5-6-g988486db9-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.08h | 0 | 7→213 | 0→231 | #4073 CONTROL2: permanent drops into BVN3 (correct partition id), reconcile OFF |
| [20260728T163801Z](20260728T163801Z/manifest.md) | `v1.4.5-6-g988486db9-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.07h | 0 | 7→169 | 0→3 | #4073 TREATMENT: permanent drops into BVN3, reconcile ON — identical to CONTROL2 |
| [20260728T164728Z](20260728T164728Z/manifest.md) | `v1.4.5-6-g988486db9-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.08h | 0 | 8→205 | 0→8 | #4073 CONTROL3: prefix permanently dropped into BVN3, reconcile OFF |
| [20260728T165623Z](20260728T165623Z/manifest.md) | `v1.4.5-6-g988486db9-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.06h | 0 | 8→142 | 0→720 | #4073 CONTROL4: ALL synthetics into BVN3 permanently dropped, reconcile OFF |
| [20260728T170348Z](20260728T170348Z/manifest.md) | `v1.4.5-6-g988486db9-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.1h | 0 | 9→246 | 0→97 | #4073 TREATMENT4: identical to CONTROL4 but reconcile ON |
| [20260728T200406Z](20260728T200406Z/manifest.md) | `v1.4.5-6-g988486db9-dirty` | v2-jiuquan | unconditional (no config, v1.4.5+) | 0.08h | 0 | 9→202 | 0→105 | #4073 TREATMENT5: same as TREATMENT4, success log now visible |

## #4073 VALIDATED in docker, 2026-07-28

Controlled A/B. Identical config and injector; only the interval reconcile differs.

| run | reconcile | drops | `→ BVN3` | stalled |
|---|---|---|---|---|
| CONTROL4 | **off** | 16 | `produced=11 received=0`, `produced=5 received=0` | **2 channels, 16 lost** |
| TREATMENT4 | on | 33 | `2/2`, `31/31` | **0** |
| TREATMENT5 | on | — | `2/2`, `54/54`, **129 reconcile pulls logged** | **0** |

With the reconcile off every message into BVN3 is permanently lost and the gap
healer cannot see it: `received=0` means no hole ever forms. With it on, all are
recovered, and TREATMENT5 shows the pulls directly —
`produced=17 received=0 requested=1`, then `received=1`, climbing to 54/54.

### Reproduction recipe

```
DROP_SYN='BVN3:%1000+999!'  TPS=2  LG_BOOTSTRAP=0  IDLE_AFTER=180
```

Three things are load-bearing, each of which silently defeated earlier attempts:

- **`!` = permanent drop.** Without it the injector produces a delay, not a loss:
  it drops each (dest, seq) once and lets the source's re-dispatch through, so
  the stream repairs itself with no healer involved. Proven by disabling the gap
  healer entirely and still seeing `received == produced`.
- **`BVN3`, not `bvn-BVN3`.** `matches()` compares the partition ID from
  `ParsePartitionUrl`, so every `bvn-*` spec used before this was a silent no-op
  and only `*` ever matched.
- **Drop essentially everything on the channel.** A prefix or tail loss with any
  later traffic behind it leaves a hole the #4064 gap healer finds and fixes, so
  the reconcile is never the mechanism under test. `received` must stay 0.

`LG_BOOTSTRAP=0` keeps channels quiet; `IDLE_AFTER` lets a loss age past
reconcileGraceBlocks after load stops.
| [20260726T051821Z](20260726T051821Z/manifest.md) | `v1.4.5` | v2-jiuquan | unconditional | 24.14h | 0 (clean) | 8→57836 | 738 syn / 4227 anc | PASS — deploy-approved; found #4073 (DN→BVN1 dead 24h, undetected) |
