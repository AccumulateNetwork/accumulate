## Your task: make these tests pass reliably, for the right reason

Package(s): `{packages}`. Declared in: `{file}`.

{tests}

These hit the 30-minute binary timeout (they hang; they do not fail):

{hung}

In CI they fail in `go test 1/2`, on every pipeline into
`dagbft-integration` since at least 2026-10-15. That includes MRs that
change only tests, so the failure belongs to the base. CI runs:

    go test -tags testnet -timeout 30m ./...    (ACC_ASSERT_IDENTITY_BOUNDARY=1)

That is every package at once, on a shared runner. A test that passes
alone on this box may still fail there under load. If yours only fails
under load, reproduce that load, for example by running alongside the rest
of the package through slots. Then decide whether it is a real
timing-dependent bug (fix it) or a budget that cannot hold on a loaded
runner (show the numbers; rule 2).

The check runs your tests by name with `-count=1`, so a pass is not
cached. Also mind the package as a whole: `pkg/consensus/consim` must
finish inside CI's 30-minute binary timeout with every test in it. Do not
spend that budget. If you make a test longer, measure it and say what the
package's total becomes.

Before you start:
- Read the failures in the CI job logs under `{ci_logs}`. Search for your
  test names and look at what each test printed before `--- FAIL`.
- Read related work in flight: #4603 / !1254 (Dennis) changes how long the
  network-restart tests run (`glab mr view 1254`, `glab mr diff 1254`), and
  #4601 lists the consensus review's findings, with issues #4590–#4600. If
  an open MR's change is the right fix for your test, cherry-pick it onto
  your branch, keep its author, and say so. Do not rewrite it.
- `consim` is the in-process full-stack simulator (see `cmd/consim` and the
  package doc). Use it to reproduce: simulation first, faster than any soak.

Each test is one network scenario, so the same root cause may break
several. Find the cause per test, and do not assume.
