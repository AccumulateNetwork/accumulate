# Research: Migrate exp/light logger

## Summary

The `exp/light` package contains a deprecated `Logger` function that accepts a `cometbft/libs/log.Logger` but does nothing with it. The package has already migrated to using `log/slog` directly for logging. The CometBFT logger import and the `Logger` function can be safely removed as they are unused.

## Verified Facts

### Fact 1: Logger function exists but is deprecated and unused
- **Source**: `exp/light/client.go:88-95`
- **Content**:
```go
// Logger sets the logger.
//
// Deprecated: Unused - using slog instead.
func Logger(logger log.Logger, keyVals ...any) ClientOption {
	return func(c *Client) error {
		return nil
	}
}
```
- **Confidence**: HIGH

### Fact 2: CometBFT log import is unused except for the deprecated Logger function
- **Source**: `exp/light/client.go:12`
- **Content**: `"github.com/cometbft/cometbft/libs/log"`
- **Confidence**: HIGH

### Fact 3: Package already uses slog for all actual logging
- **Source**: `exp/light/sync.go:14` and multiple lines throughout
- **Content**:
  - Import: `"log/slog"`
  - Usage: `slog.InfoContext(ctx, ...)` at lines 140, 194, 326, 422, 445, 534, 542, 614, 768, 824, 969
  - Usage: `slog.ErrorContext(ctx, ...)` at line 194
- **Confidence**: HIGH

### Fact 4: FromCometBFT wrapper exists in internal/logging
- **Source**: `internal/logging/compat.go:25-35`
- **Content**:
```go
// FromCometBFT wraps a cometbft/libs/log.Logger as our Logger interface.
func FromCometBFT(l log.Logger) Logger {
	if l == nil {
		return Nop{}
	}
	// Check if it's a cometAdapter, return the underlying logger
	if a, ok := l.(*cometAdapter); ok {
		return a.l
	}
	return &fromCometAdapter{l}
}
```
- **Confidence**: HIGH

### Fact 5: Client struct does not store a logger
- **Source**: `exp/light/client.go:29-35`
- **Content**:
```go
type Client struct {
	v2          *client.Client
	query       api.Querier2
	store       keyvalue.Beginner
	storePrefix string
	router      routing.Router
}
```
- **Confidence**: HIGH

### Fact 6: Other packages demonstrate proper migration pattern
- **Source**: `exp/apiutil/p2p.go:41`, `cmd/accumulated/run/api.go:63`
- **Content**:
  - `logger := logging.FromCometBFT(opts.Logger)` - shows proper wrapper usage
  - These packages actually use the wrapped logger, unlike exp/light which ignores it
- **Confidence**: HIGH

## Code References

### Primary Files
- `exp/light/client.go` - Contains deprecated Logger function (lines 88-95) and unused CometBFT import (line 12)
- `exp/light/sync.go` - Shows actual slog usage throughout

### Related Logging Infrastructure
- `internal/logging/compat.go` - Contains `FromCometBFT()` wrapper (lines 25-35)
- `internal/logging/logger.go` - Defines `Logger` interface (lines 14-19)

## Open Questions

None. The migration path is clear: remove the unused CometBFT logger dependency entirely since the package already uses slog.

## Contradictions

None. The issue title mentions "use logging.FromCometBFT() wrapper" but the actual situation is that:
1. The Logger function is marked deprecated
2. The Logger function does nothing (returns nil error, stores nothing)
3. The package already uses slog directly
4. The correct fix is to simply remove the unused code rather than wire up a wrapper

## Recommended Implementation

The implementation is simpler than the issue description suggests:

1. Remove the `"github.com/cometbft/cometbft/libs/log"` import from `exp/light/client.go:12`
2. Remove the deprecated `Logger` function from `exp/light/client.go:88-95`

No wrapper migration is needed because:
- The package does not use the passed logger
- All logging is already done via `log/slog`
- There's no Logger field in the Client struct to store a logger

If the Logger option must be preserved for API compatibility, it could be changed to accept `*slog.Logger` or simply left as deprecated (current state is acceptable since it's a no-op).
