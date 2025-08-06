# Test Debugging Guide

## Table of Contents

1. [Overview](#overview)
2. [Quick Start](#quick-start)
3. [Debugging Tools](#debugging-tools)
4. [VS Code Integration](#vs-code-integration)
5. [Command Line Debugging](#command-line-debugging)
6. [Test-Specific Debugging](#test-specific-debugging)
7. [Common Issues](#common-issues)
8. [Logging and Tracing](#logging-and-tracing)
9. [Performance Debugging](#performance-debugging)
10. [Best Practices](#best-practices)

## Overview

Debugging tests effectively is crucial for maintaining a reliable test suite. This guide covers debugging techniques, tools, and strategies for the Accumulate Network test suite.

### Debugging Philosophy

- **Isolate the Problem**: Run minimal test cases to identify root cause
- **Use Appropriate Tools**: Choose the right debugging tool for the situation
- **Systematic Approach**: Follow a structured debugging process
- **Document Findings**: Record solutions for future reference

## Quick Start

### Basic Debugging Commands

```bash
# Run single test with verbose output
go test -v -run TestSpecificTest ./pkg/database

# Run with race detection
go test -race -run TestSpecificTest ./internal/api

# Run with detailed output
go test -v -count=1 -run TestSpecificTest ./test/e2e

# Debug with custom timeout
go test -timeout=30m -v -run TestLongRunning ./test/simulator
```

### Emergency Debugging

```bash
# When tests hang
pkill -f "go test"
ps aux | grep "go test"

# When tests consume too much memory
go test -memprofile=mem.prof -run TestMemoryIssue ./...
go tool pprof mem.prof

# When tests are flaky
go test -count=100 -run TestFlaky ./...
```

## Debugging Tools

### 1. Go Testing Framework

#### Verbose Output
```bash
# Basic verbose output
go test -v ./...

# With test function timing
go test -v -test.v ./...

# With parallel execution details
go test -v -parallel=4 ./...
```

#### Test Selection
```bash
# Run specific test
go test -run TestSpecificFunction ./pkg/database

# Run tests matching pattern
go test -run "TestDatabase.*" ./pkg/database

# Run specific subtest
go test -run "TestDatabase/subtest_name" ./pkg/database

# Skip specific tests
go test -skip "TestSlow.*" ./...
```

#### Test Flags
```bash
# Disable test caching
go test -count=1 ./...

# Set custom timeout
go test -timeout=10m ./...

# Run with race detector
go test -race ./...

# Generate coverage
go test -cover ./...
go test -coverprofile=coverage.out ./...
```

### 2. Delve Debugger

#### Installation
```bash
go install github.com/go-delve/delve/cmd/dlv@latest
```

#### Basic Usage
```bash
# Debug specific test
dlv test ./pkg/database -- -test.run TestSpecificTest

# Debug with arguments
dlv test ./test/e2e -- -test.run TestE2E -test.v

# Debug running process
dlv attach $(pgrep accumulated)
```

#### Delve Commands
```bash
# Set breakpoint
(dlv) break main.TestFunction
(dlv) break /path/to/file.go:123

# Continue execution
(dlv) continue
(dlv) c

# Step through code
(dlv) next
(dlv) step
(dlv) stepout

# Inspect variables
(dlv) print variableName
(dlv) locals
(dlv) args

# View stack trace
(dlv) stack
(dlv) goroutines
```

### 3. Built-in Debugging

#### Test Helper Functions
```go
func TestWithDebugging(t *testing.T) {
    // Enable verbose logging
    if testing.Verbose() {
        t.Logf("Debug: Starting test")
    }
    
    // Skip in short mode
    if testing.Short() {
        t.Skip("Skipping in short mode")
    }
    
    // Parallel execution
    t.Parallel()
    
    // Cleanup
    t.Cleanup(func() {
        t.Logf("Debug: Cleaning up")
    })
}
```

#### Custom Debug Output
```go
func debugPrint(t *testing.T, format string, args ...interface{}) {
    if testing.Verbose() {
        t.Logf("DEBUG: "+format, args...)
    }
}

func TestWithCustomDebug(t *testing.T) {
    debugPrint(t, "Starting test with value: %v", someValue)
    
    // Test implementation
    result := someFunction()
    debugPrint(t, "Function returned: %v", result)
    
    assert.Equal(t, expected, result)
}
```

## VS Code Integration

### Launch Configuration

Create `.vscode/launch.json`:

```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Debug Test",
            "type": "go",
            "request": "launch",
            "mode": "test",
            "program": "${workspaceFolder}/pkg/database",
            "args": [
                "-test.run",
                "TestSpecificTest",
                "-test.v"
            ],
            "env": {
                "ACC_LOG_LEVEL": "debug"
            }
        },
        {
            "name": "Debug E2E Test",
            "type": "go",
            "request": "launch",
            "mode": "test",
            "program": "${workspaceFolder}/test/e2e",
            "args": [
                "-test.run",
                "TestE2EWorkflow",
                "-test.v",
                "-test.timeout",
                "30m"
            ],
            "env": {
                "ACC_TEST_TIMEOUT": "30m",
                "ACC_LOG_LEVEL": "debug"
            }
        },
        {
            "name": "Debug Simulator Test",
            "type": "go",
            "request": "launch",
            "mode": "test",
            "program": "${workspaceFolder}/test/simulator",
            "args": [
                "-test.run",
                "TestSimulator",
                "-test.v"
            ]
        },
        {
            "name": "Debug Load Test",
            "type": "go",
            "request": "launch",
            "mode": "debug",
            "program": "${workspaceFolder}/test/cmd/load/main.go",
            "args": [
                "-server=http://localhost:8080",
                "-transactions=100",
                "-duration=60"
            ]
        }
    ]
}
```

### VS Code Tasks

Create `.vscode/tasks.json`:

```json
{
    "version": "2.0.0",
    "tasks": [
        {
            "label": "Run Test with Debug",
            "type": "shell",
            "command": "go",
            "args": [
                "test",
                "-v",
                "-run",
                "${input:testName}",
                "${input:packagePath}"
            ],
            "group": "test",
            "presentation": {
                "echo": true,
                "reveal": "always",
                "focus": false,
                "panel": "shared"
            }
        }
    ],
    "inputs": [
        {
            "id": "testName",
            "description": "Test name pattern",
            "default": "TestSpecific",
            "type": "promptString"
        },
        {
            "id": "packagePath",
            "description": "Package path",
            "default": "./pkg/database",
            "type": "promptString"
        }
    ]
}
```

### Debugging Workflow

1. **Set Breakpoints**: Click in the gutter or press F9
2. **Start Debugging**: Press F5 or use Command Palette
3. **Step Through Code**: Use F10 (step over), F11 (step into)
4. **Inspect Variables**: Hover over variables or use Debug Console
5. **Evaluate Expressions**: Use Debug Console for custom expressions

## Command Line Debugging

### Environment Variables

```bash
# Enable debug logging
export ACC_LOG_LEVEL=debug
export ACC_LOG_FORMAT=text

# Test-specific variables
export ACC_TEST_TIMEOUT=30m
export ACC_TEST_VERBOSE=true
export ACC_TEST_PARALLEL=false

# Database debugging
export ACC_DB_DEBUG=true
export ACC_DB_LOG_QUERIES=true

# Network debugging
export ACC_NET_DEBUG=true
export ACC_NET_LOG_MESSAGES=true
```

### Debug-Specific Test Runs

```bash
# Run with maximum verbosity
go test -v -x -a -work ./...

# Run with build information
go test -v -ldflags="-X main.version=debug" ./...

# Run with custom build tags
go test -tags=debug -v ./...

# Run with environment setup
ACC_LOG_LEVEL=debug go test -v -run TestSpecific ./...
```

### Debugging Hanging Tests

```bash
# Run with timeout and stack trace on timeout
go test -timeout=30s -v ./... 2>&1 | tee test.log

# Send SIGQUIT to get stack traces
kill -QUIT $(pgrep "go test")

# Use strace to see system calls (Linux)
strace -p $(pgrep "go test")

# Use timeout command
timeout 60s go test -v ./...
```

## Test-Specific Debugging

### Unit Test Debugging

```go
func TestWithDebugInfo(t *testing.T) {
    // Setup debug context
    ctx := context.WithValue(context.Background(), "debug", true)
    
    // Create test subject with debug enabled
    subject := NewSubject(WithDebug(true))
    
    // Test with debug output
    result, err := subject.Process(ctx, input)
    
    // Debug assertions
    if testing.Verbose() {
        t.Logf("Input: %+v", input)
        t.Logf("Result: %+v", result)
        t.Logf("Error: %v", err)
    }
    
    require.NoError(t, err)
    assert.Equal(t, expected, result)
}
```

### E2E Test Debugging

```go
func TestE2EWithDebug(t *testing.T) {
    // Enable debug mode
    harness := NewHarness(t).WithDebug(true)
    defer harness.Close()
    
    // Start services with debug logging
    harness.StartServices(WithLogLevel("debug"))
    
    // Create debug client
    client := harness.Client().WithDebug(true)
    
    // Execute with debug tracing
    result, err := client.Execute(request)
    
    // Debug output
    if testing.Verbose() {
        t.Logf("Request: %+v", request)
        t.Logf("Response: %+v", result)
        
        // Dump service logs
        harness.DumpLogs(t)
    }
    
    require.NoError(t, err)
}
```

### Simulator Test Debugging

```go
func TestSimulatorWithDebug(t *testing.T) {
    // Create simulator with debug enabled
    sim := simulator.New(t, 3).WithDebug(true)
    defer sim.Close()
    
    // Enable step-by-step execution
    sim.SetStepMode(true)
    
    sim.InitFromGenesis()
    
    // Debug each step
    for i := 0; i < 10; i++ {
        t.Logf("Executing step %d", i)
        
        sim.ExecuteBlock()
        
        // Inspect state
        state := sim.GetState()
        t.Logf("State after step %d: %+v", i, state)
        
        // Check for issues
        if sim.HasErrors() {
            errors := sim.GetErrors()
            t.Logf("Errors detected: %v", errors)
        }
    }
}
```

## Common Issues

### 1. Flaky Tests

#### Identification
```bash
# Run test multiple times
go test -count=100 -run TestFlaky ./...

# Run with race detector
go test -race -count=10 -run TestFlaky ./...

# Run in parallel
go test -parallel=10 -count=50 -run TestFlaky ./...
```

#### Common Causes and Solutions

```go
// Problem: Race condition
func TestRaceCondition(t *testing.T) {
    var counter int
    
    go func() {
        counter++ // Race!
    }()
    
    assert.Equal(t, 1, counter) // Flaky!
}

// Solution: Proper synchronization
func TestWithSynchronization(t *testing.T) {
    var counter int
    var mu sync.Mutex
    var wg sync.WaitGroup
    
    wg.Add(1)
    go func() {
        defer wg.Done()
        mu.Lock()
        counter++
        mu.Unlock()
    }()
    
    wg.Wait()
    
    mu.Lock()
    result := counter
    mu.Unlock()
    
    assert.Equal(t, 1, result)
}
```

### 2. Timeout Issues

```go
// Problem: No timeout handling
func TestWithoutTimeout(t *testing.T) {
    result := longRunningOperation() // May hang forever
    assert.NotNil(t, result)
}

// Solution: Context with timeout
func TestWithTimeout(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    result, err := longRunningOperationWithContext(ctx)
    require.NoError(t, err)
    assert.NotNil(t, result)
}
```

### 3. Resource Leaks

```go
// Problem: Resource not cleaned up
func TestResourceLeak(t *testing.T) {
    db := openDatabase()
    // Missing: defer db.Close()
    
    result := db.Query("SELECT * FROM table")
    assert.NotNil(t, result)
}

// Solution: Proper cleanup
func TestWithCleanup(t *testing.T) {
    db := openDatabase()
    defer db.Close() // Always cleanup
    
    // Or use t.Cleanup
    t.Cleanup(func() {
        db.Close()
    })
    
    result := db.Query("SELECT * FROM table")
    assert.NotNil(t, result)
}
```

### 4. Environment Dependencies

```go
// Problem: Environment-dependent test
func TestEnvironmentDependent(t *testing.T) {
    file := "/tmp/specific-file" // May not exist
    data, err := os.ReadFile(file)
    require.NoError(t, err)
    assert.NotEmpty(t, data)
}

// Solution: Setup test environment
func TestWithSetup(t *testing.T) {
    // Create temporary file
    tmpFile, err := os.CreateTemp("", "test-*.txt")
    require.NoError(t, err)
    defer os.Remove(tmpFile.Name())
    
    // Write test data
    testData := []byte("test content")
    _, err = tmpFile.Write(testData)
    require.NoError(t, err)
    tmpFile.Close()
    
    // Test with known file
    data, err := os.ReadFile(tmpFile.Name())
    require.NoError(t, err)
    assert.Equal(t, testData, data)
}
```

## Logging and Tracing

### Structured Logging

```go
import (
    "log/slog"
    "os"
)

func TestWithStructuredLogging(t *testing.T) {
    // Create debug logger
    logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelDebug,
    }))
    
    // Use in test
    logger.Debug("Starting test", "test", t.Name())
    
    subject := NewSubject(WithLogger(logger))
    result, err := subject.Process(input)
    
    logger.Debug("Test completed", 
        "result", result,
        "error", err,
        "duration", time.Since(start))
    
    require.NoError(t, err)
}
```

### Trace Integration

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/trace"
)

func TestWithTracing(t *testing.T) {
    // Create tracer
    tracer := otel.Tracer("test")
    
    // Start span
    ctx, span := tracer.Start(context.Background(), "test-operation")
    defer span.End()
    
    // Add attributes
    span.SetAttributes(
        attribute.String("test.name", t.Name()),
        attribute.String("test.package", "pkg/database"),
    )
    
    // Execute with tracing
    result, err := subject.ProcessWithContext(ctx, input)
    
    // Record result
    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
    } else {
        span.SetStatus(codes.Ok, "success")
    }
    
    require.NoError(t, err)
}
```

## Performance Debugging

### Memory Profiling

```bash
# Generate memory profile
go test -memprofile=mem.prof -run TestMemoryIntensive ./...

# Analyze profile
go tool pprof mem.prof

# Interactive analysis
(pprof) top10
(pprof) list FunctionName
(pprof) web
```

### CPU Profiling

```bash
# Generate CPU profile
go test -cpuprofile=cpu.prof -run TestCPUIntensive ./...

# Analyze profile
go tool pprof cpu.prof

# Generate flame graph
go tool pprof -http=:8080 cpu.prof
```

### Benchmark Debugging

```go
func BenchmarkWithProfiling(b *testing.B) {
    // Setup
    data := setupBenchmarkData()
    
    // Reset timer after setup
    b.ResetTimer()
    
    // Enable memory stats
    b.ReportAllocs()
    
    for i := 0; i < b.N; i++ {
        result := processData(data)
        
        // Prevent optimization
        _ = result
    }
}
```

## Best Practices

### 1. Systematic Debugging

```go
func TestSystematicDebugging(t *testing.T) {
    // 1. Document the problem
    t.Logf("Testing: %s", "specific functionality")
    t.Logf("Expected: %v", expectedResult)
    
    // 2. Isolate the issue
    input := createMinimalInput()
    
    // 3. Add debug output
    if testing.Verbose() {
        t.Logf("Input: %+v", input)
    }
    
    // 4. Execute and capture
    result, err := subject.Process(input)
    
    // 5. Analyze results
    if testing.Verbose() {
        t.Logf("Result: %+v", result)
        t.Logf("Error: %v", err)
    }
    
    // 6. Assert and document
    require.NoError(t, err, "Process should not fail")
    assert.Equal(t, expectedResult, result, "Result should match expected")
}
```

### 2. Debug-Friendly Test Design

```go
// Good: Debuggable test
func TestDebuggable(t *testing.T) {
    // Clear test data
    input := TestInput{
        Field1: "value1",
        Field2: 42,
    }
    
    expected := TestOutput{
        Result: "processed_value1_42",
    }
    
    // Single responsibility
    result := processInput(input)
    
    // Clear assertion
    assert.Equal(t, expected, result)
}

// Bad: Hard to debug
func TestHardToDebug(t *testing.T) {
    // Complex setup
    input := createComplexInput()
    
    // Multiple operations
    intermediate := processStep1(input)
    result := processStep2(intermediate)
    
    // Unclear assertion
    assert.True(t, result.IsValid())
}
```

### 3. Error Context

```go
func TestWithErrorContext(t *testing.T) {
    input := createTestInput()
    
    result, err := subject.Process(input)
    
    // Provide context in assertions
    require.NoError(t, err, "Process failed with input: %+v", input)
    
    assert.NotNil(t, result, "Result should not be nil for input: %+v", input)
    
    assert.Equal(t, expected, result, 
        "Result mismatch for input: %+v, got: %+v, want: %+v", 
        input, result, expected)
}
```

### 4. Reproducible Debugging

```go
func TestReproducible(t *testing.T) {
    // Use fixed seed for randomness
    rand.Seed(12345)
    
    // Use fixed time
    fixedTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
    
    // Create deterministic input
    input := createDeterministicInput(fixedTime)
    
    result := subject.Process(input)
    
    // Test will be reproducible
    assert.Equal(t, expectedResult, result)
}
```

---

## See Also

- [testing.md](testing.md) - Complete testing guide
- [unit-tests.md](unit-tests.md) - Unit testing guide
- [e2e-tests.md](e2e-tests.md) - End-to-end testing guide
- [performance-tests.md](performance-tests.md) - Performance testing guide
- [ci-cd.md](ci-cd.md) - CI/CD integration guide

*This guide focuses on debugging techniques and troubleshooting. For test implementation guidance, see the related documentation.*
