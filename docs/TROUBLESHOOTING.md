# Accumulate Troubleshooting Guide

## Common Build and Compilation Issues

### The "package 2 is not in std" Error

#### Problem Description
When building Go code, you may encounter a cryptic error message:
```
package 2 is not in std (/usr/local/go/src/2)
```

This error is particularly confusing because:
- It doesn't indicate what the actual problem is
- The error message suggests Go is trying to import a package named "2"
- Running the same command multiple times produces the same error
- The real compilation errors are completely hidden

#### Root Cause
This error occurs when:
1. There are actual compilation errors in your Go code (missing functions, type mismatches, etc.)
2. You're using shell redirection `2>&1` in your build command
3. The shell or Go toolchain misinterprets the redirection syntax

The "2" in the error message comes from the `2>&1` stderr redirection being incorrectly parsed as a package import.

#### Common Scenarios Where This Occurs
```bash
# These commands may trigger the error:
go build ./path/to/package 2>&1 | head -50
go build . 2>&1 | grep error
go list ./... 2>&1
```

#### Solution

##### Step 1: Get the Real Error Messages
Instead of using shell redirection with pipes, try these approaches:

```bash
# Option 1: Run without redirection first
go build ./path/to/package

# Option 2: Use go run to see actual errors
go run ./path/to/package

# Option 3: Build with verbose output
go build -v ./path/to/package

# Option 4: If you must use redirection, try parentheses
(go build ./path/to/package) 2>&1
```

##### Step 2: Common Underlying Issues to Check

Once you can see the real errors, look for:

1. **Missing build tags or ignored files**
   ```go
   //go:build ignore  // Remove this line if the file should be compiled
   ```

2. **Generic type instantiation problems**
   ```go
   // Wrong: Type alias without instantiation
   type Promise = promise.Promise  // Missing [T any]
   
   // Correct:
   type Promise[T any] = promise.Promise[T]
   ```

3. **Missing helper functions**
   - Check if all imported functions actually exist
   - Verify function signatures match their usage

4. **Import cycles or missing imports**
   ```go
   // Check for accidental import of non-existent packages
   import "2"  // This would directly cause the error
   ```

#### Minimal Reproducible Example

A complete minimal example that demonstrates this error is available in:
```
docs/package-2-error-example/
```

To run the example:
```bash
cd docs/package-2-error-example
./reproduce-error.sh
```

This will show you:
1. How the "package 2" error appears with shell redirection
2. The actual compilation errors being hidden
3. How to properly diagnose the issue

#### Example Case Study

**Symptoms:**
```bash
$ go build ./tools/cmd/debug 2>&1 | head -20
package 2 is not in std (/usr/local/go/src/2)
```

**Actual Problem (hidden by the error):**
```go
// network.go was missing these helper functions:
func maybe[T any](fn func() (T, bool)) func() promise.Result[T]
func waitFor[T any](wg *sync.WaitGroup, promise promise.Promise[T])
func done[T any](fn func(T)) func(T) promise.Result[any]
```

**Solution:**
1. Created `network_vars.go` with the missing helper functions
2. Removed `//go:build ignore` tags from related files
3. Fixed generic type instantiation issues

#### Prevention

1. **Avoid complex shell redirections when debugging build issues**
   - First run commands without any redirection
   - Add pipes and redirections only after confirming the build works

2. **Use proper error checking in build scripts**
   ```bash
   # Good: Check error before piping
   if go build ./package; then
       go build ./package 2>&1 | process_output
   fi
   
   # Bad: Immediate piping can mask errors
   go build ./package 2>&1 | process_output
   ```

3. **Enable verbose output during debugging**
   ```bash
   go build -v -x ./package  # Shows what Go is actually doing
   ```

4. **Check Go version compatibility**
   ```bash
   go version  # Ensure you're using the expected version
   go mod tidy # Clean up module dependencies
   ```

#### Related Issues

- Generic type aliases require Go 1.23 or later
- Build tags must have no space after `//`
- Shell interpretations vary between bash, zsh, and other shells

#### When to Seek Help

If you've tried all the above solutions and still see the "package 2" error:
1. Try building in a different shell (bash vs zsh)
2. Check for unusual characters in your source files
3. Verify your Go installation is not corrupted
4. Create a minimal reproducible example and file an issue

---

## Other Common Issues

### P2P Connection Failures

[Previous documentation about P2P issues would go here]

### Bootstrap Server Configuration

[Previous documentation about bootstrap servers would go here]