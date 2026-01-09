# Accumulate Development Guide for Claude Code

This file provides context for AI assistants working on the Accumulate codebase.

## Build System (CRITICAL)

### Always Use Make

**NEVER use `go build` or `go install` directly.** Binaries built without `make` will not have version information embedded.

```bash
# Correct - embeds version from git tags
make build                    # Build accumulated binary

# Wrong - produces unversioned binary
go build ./cmd/accumulated    # DO NOT USE
```

### Version Format

Versions use git-describe: `v<tag>-<commits>-g<hash>`

Examples:
- `v1.4.4` - exactly at tag
- `v1.4.4-beta.3-8-g40b460aa7` - 8 commits after v1.4.4-beta.3
- `v1.4.4-dirty` - uncommitted changes

### Check Version

```bash
# Verify version is embedded
./accumulated version
# Should show: Accumulate network daemon MainNet v1.4.4-beta.3-8-g40b460aa7

# If it shows "version unknown", rebuild with make
```

### Version Enforcement

Binaries without proper version information should be rejected:
- `version unknown` - binary built with `go build` instead of `make`
- `$(git describe --dirty)` - ldflags not applied
- Empty version string

The `accman-superv` supervisor validates accumulated version before starting it.

---

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

---

## Code Style

- Follow Go naming conventions
- Use structured logging (no fmt.Print statements in production code)
- Add `//nolint` directives with explanatory comments when suppressing lint warnings
- Don't name files in all caps (except standard files like README.md, CHANGELOG.md)

---

## Git Workflow

- Branch naming: `<issue-number>-<short-description>`
- Reference issues in commit messages with `#<issue-number>`

---

## Key Directories

| Directory | Purpose |
|-----------|---------|
| `cmd/accumulated/` | Main node binary |
| `cmd/accumulated/run/` | Node runtime, consensus |
| `protocol/` | Protocol types, transactions |
| `pkg/api/` | API implementations |
| `internal/node/` | Node internals |
| `mcp/` | MCP server for blockchain operations |

---

## Important Patterns

### Version Variables

```go
// In version.go - set via ldflags at build time
var Version = "version unknown"  // Default if not built with make
var Commit string
```

### Makefile ldflags

```makefile
GIT_DESCRIBE = $(shell git fetch --tags -q ; git describe --dirty)
GIT_COMMIT = $(shell git rev-parse HEAD)
LDFLAGS = '-X "gitlab.com/accumulatenetwork/accumulate.Version=$(GIT_DESCRIBE)" -X "gitlab.com/accumulatenetwork/accumulate.Commit=$(GIT_COMMIT)"'
```

---

## Output Handling

When running commands that produce verbose output, redirect to log files:

```bash
# Good - prevents context overflow
make build > /tmp/build.log 2>&1

# Bad - streams verbose output
make build
```

---

## Related Projects

| Project | Purpose |
|---------|---------|
| `accman` | Accumulate Manager - follower node management, supervisor daemon |
| `accumulate` | Core protocol implementation |

See `accman/CLAUDE.md` for supervisor and deployment requirements.
