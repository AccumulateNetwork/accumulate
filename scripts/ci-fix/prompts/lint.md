## Your task: make CI's `lint` job pass (scope: {scope})

These findings are from CI:

{findings}

The check runs what CI's lint job runs (`.gitlab/all.gitlab-ci.yml`, job
`lint`):
1. `go run ./tools/cmd/golangci-lint run`, configured by `.golangci.yml`
2. then `go mod tidy`, `go generate ./...` and `gosimports -l`, each of
   which must leave the tree unchanged

CI's lint job stops at its first failing step, so CI showed only the
golangci-lint findings above. Run the check: on the base, the steps after
golangci-lint fail too (untidy `go.mod`, stale generated files, files that
are not gosimports-formatted). Fix all of them. For a generated file that
is stale, find out why before you regenerate it. Was a generated file
edited by hand? Did a generator input change without regenerating? Did the
generator itself change? Regenerating over a hand edit can silently revert
a fix, so if you find one, move the edit into the generator's input and say
so in the commit. Format with `go run github.com/rinchsan/gosimports/cmd/gosimports -w`
on the listed files only, or `./scripts/imports.sh`. Commit formatting
apart from everything else, so a reviewer can skip it.

Scope `changed` means this is an open MR branch. Fix only findings in files
this branch changes relative to `{base}`; the base's own findings are
another task's. Scope `all` means every finding.

Fix each finding properly:
- **errcheck:** handle the error. Do not discard it with `_ =` or a
  `//nolint`, unless ignoring it is genuinely correct, and then the comment
  says why. In a command-line tool, a bad argument is an error to report.
- **goheader:** the header template is in `.golangci.yml`. Look at how the
  linter decides the expected year before you change any file.

Keep the change to what lint needs. For scope `changed` this is small: make the change, run the check, commit,
and report done.
