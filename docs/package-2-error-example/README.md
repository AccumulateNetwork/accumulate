# Minimal Example: "package 2 is not in std" Error

This directory contains a minimal reproducible example of the confusing "package 2 is not in std" error that can occur when building Go code with shell redirection.

## Files

- `main.go` - Intentionally broken Go code with compilation errors
- `reproduce-error.sh` - Script that demonstrates how the error occurs
- `README.md` - This file

## How to Reproduce

1. Navigate to this directory:
   ```bash
   cd docs/package-2-error-example
   ```

2. Try to build with redirection (this may produce the cryptic error):
   ```bash
   go build . 2>&1 | head -5
   ```
   
   You might see:
   ```
   package 2 is not in std (/usr/local/go/src/2)
   ```

3. Build without redirection to see the real errors:
   ```bash
   go build .
   ```
   
   Now you'll see the actual problems:
   ```
   ./main.go:11:12: undefined: nonExistentFunction
   ./main.go:15:14: undefined: undefinedVariable
   ./main.go:18:2: undefined: anotherMissingFunction
   ```

## Running the Demo Script

```bash
chmod +x reproduce-error.sh
./reproduce-error.sh
```

## The Problem

The issue occurs when:
1. Your Go code has compilation errors
2. You use `2>&1` to redirect stderr to stdout
3. You pipe the output to another command
4. The shell or Go toolchain misinterprets the "2" as a package import

## Why This Example?

This minimal example contains:
- Simple, obvious compilation errors (undefined functions and variables)
- No external dependencies
- Clear demonstration of how shell redirection masks the real errors

## Key Takeaway

**Always run `go build` without redirection first when debugging compilation issues.**

The "package 2" error is not the real problem - it's a symptom of shell redirection interfering with error reporting when there are actual compilation errors in your code.

## Variations That Can Trigger This

```bash
# All of these might produce the "package 2" error:
go build . 2>&1 | grep error
go test ./... 2>&1 | head
go list ./... 2>&1
go run . 2>&1 | tee output.log
```

## Solution Pattern

```bash
# Step 1: Run without redirection
go build .

# Step 2: Fix the actual compilation errors shown

# Step 3: Now you can safely use redirection if needed
go build . 2>&1 | your-processing-script
```