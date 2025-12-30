# Accumulate Development Guide for Claude Code

## Before Committing

Run the following commands before committing changes:

### Format imports
```bash
go run github.com/rinchsan/gosimports/cmd/gosimports -w .
```

### Run linting
```bash
golangci-lint run
```

### Run tests
```bash
go test ./...
```

## Code Style

- Follow Go naming conventions
- Use structured logging (no fmt.Print statements in production code)
- Add `//nolint` directives with explanatory comments when suppressing lint warnings

## Git Workflow

- Branch naming: `<issue-number>-<short-description>`
- Reference issues in commit messages with `#<issue-number>`
