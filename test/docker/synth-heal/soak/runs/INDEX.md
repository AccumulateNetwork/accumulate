# Soak runs

Every run appends one row. Details in `<runId>/manifest.md`.

| run | commit | executor | healing | elapsed | exit | dn height | heals | note |
|---|---|---|---|---|---|---|---|---|
| [20260725T233615Z](20260725T233615Z/manifest.md) | `v1.4.4.2-20-ga048e0035` | v2-jiuquan | synthetic+anchor | 4.96h of 24h | STOPPED | 7→12230 | 407 syn / 495 anc | stopped early for the v1.4.5 build; no wedge in ~5h |
| [20260726T051821Z](20260726T051821Z/manifest.md) | `v1.4.5` | v2-jiuquan | unconditional | 24.14h | 0 (clean) | 8→57836 | 738 syn / 4227 anc | PASS — deploy-approved; found #4073 (DN→BVN1 dead 24h, undetected) |
