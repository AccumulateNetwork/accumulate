## Your task: make CI's `lint` job pass (scope: {scope})

These findings are from CI:

{findings}

The check runs what CI's lint job runs (`.gitlab/all.gitlab-ci.yml`, job
`lint`):
1. `go run ./tools/cmd/golangci-lint run`, configured by `.golangci.yml`
2. then `go mod tidy`, `go generate ./...` and `gosimports -l`, each of
   which must leave the tree unchanged

Scope `changed` means this is an open MR branch. Fix only findings in files
this branch changes relative to `{base}`; the base's own findings are
another task's. Scope `all` means every finding.

Fix each finding properly:
- **errcheck:** handle the error. Do not discard it with `_ =` or a
  `//nolint`, unless ignoring it is genuinely correct, and then the comment
  says why. In a command-line tool, a bad argument is an error to report.
- **goheader:** the header template is in `.golangci.yml`. Look at how the
  linter decides the expected year before you change any file.

Keep the change to what lint needs. This is a small task: make the change,
run the check, commit, and report done.
