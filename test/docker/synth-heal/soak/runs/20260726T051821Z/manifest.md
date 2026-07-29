# Soak run 20260726T051821Z

**Purpose:** v1.4.5 release image: #4070 hold-for-anchor + unconditional conductor healing at v2-jiuquan

| field | value |
|---|---|
| started (UTC) | 2026-07-26T05:18:21Z |
| commit | `9d336d19c661ee7cf6a17d2f2aa673b84890fda1` |
| describe | `v1.4.5-2-g9d336d19c` |
| branch | `merge/release-1.4.4.2-into-main` |
| uncommitted files | 0  |
| image | `acc-release:v1.4.5` |
| image id | `sha256:49ec5e30274d6f23b2fec73e98763b0cd146459ce8e45dbd60a730331089afda` |
| executor version | **v2-jiuquan** |
| healing | unconditional (no config, v1.4.5+) |
| synthetic drops | `*:%499+3` |
| anchor drops | `*:%997+3` |
| topology | 3 BVNs, 12 nodes + bootstrap |
| target duration | 24h |
| target TPS | 2 |

Config as run is frozen in `config/`. Results appended below on exit.

## Result

| field | value |
|---|---|
| ended (UTC) | 2026-07-27T05:28:10Z |
| elapsed | 24.14h |
| driver exit | 0 (clean) |
| dn height | 8 -> 57836 |
| heals | 0 -> 4682 |
| chaos events | 131 |
| monitor samples | 287 |
| seizure | none detected |

Raw: `soak.log`, `monitor.csv`, `chaos.log`, `loadgen-stats.json`.

### Known defect found by this run — #4073

**"seizure: none detected" above is not a clean bill of health.** The DN→BVN1
synthetic stream was dead for the entire 24 hours and no check in the harness
saw it:

```
DN   ledger, entry for BVN1 :  produced = 2
BVN1 ledger, entry for dn   :  received = None, delivered = None
```

The injector dropped both messages of that stream at 05:19:07 and 05:19:37, two
minutes in. Nothing ever followed them, so the destination formed no pending
window, `Received` never passed `Delivered`, and both the healer and the
monitor's `gap = received - delivered` read healthy — `0 - 0 = 0` — for a full
day. Filed as **#4073**; fixed by the interval reconcile on branch
`4073-idle-stream-reconcile` (!1165), which is NOT in the image this run tested.

`final-soakmon.json` records the end state under the new `undeliv` metric
(`produced - received`), which is what exposes it. Six cells were non-zero at the
end; five were 1–3 messages in flight, normal at shutdown. DN→BVN1 at
`produced=2 received=0` is 100% of the stream and had been that way since minute
two — that is the shape to look for.

### Verdict

**Good enough to deploy** (Paul, 2026-07-27). 24h at `v2-jiuquan` on the release
image, 131 chaos events, 738 synthetic and 4227 anchor heals, 160k transactions
at 12 rejections, 7 stranded against a tolerance of 20, no wedge on any stream
carrying continuous traffic, no crash, no consensus stall.

Deploying with #4073 open is a deliberate call, and the exposure is bounded: it
affects streams quiet enough that no message follows a loss. On mainnet that is
DN↔Cyclops administrative traffic, not user transactions, and it is the same
class as #4059. The fix carries no activation gate, so it can ship in a patch
release without coordination.
