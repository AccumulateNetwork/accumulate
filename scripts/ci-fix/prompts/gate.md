## Your task: the gate on the fix branch `{branch}`

Every task's branch was validated on its own. The monitor has merged them
(or tried to) into `{branch}`. The gate is the whole thing together:

- the full lint job, and
- every touched package run whole, as CI runs it: `{packages}`, with
  `-tags testnet -timeout 30m`

After the gate passes locally, the monitor pushes `{branch}` and waits for
CI. If CI is red, you are started (or messaged) with the failing jobs and
their logs.

{extra}

What to do:
- **A merge conflict:** finish the merge by hand so every task's fix holds.
  The task branches are `{base}`-based branches named
  `issue-{issue}-ci-green-<task>`. Read each side's commit message for its
  root cause before resolving.
- **A failure that no single task saw** (two fixes interacting, the package
  overrunning 30 minutes, a test outside every task): find its cause like
  any other (rules 1–3). The package's time budget counts. Measure each
  test's duration from `go test -v` output.
- **A CI-only failure:** the runner is more loaded than this box. Compare
  the CI log's per-test timings with your local ones before you decide
  anything.

Commit on `{branch}` itself, then report done. The monitor runs the gate
again, pushes, and waits for CI.
