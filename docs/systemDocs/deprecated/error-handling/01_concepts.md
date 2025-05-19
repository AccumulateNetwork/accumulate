# Error Handling Approaches

```yaml
# AI-METADATA
document_type: concept
project: accumulate_network
component: error_handling
version: current
key_concepts:
  - error_taxonomy
  - fallback_mechanisms
  - retry_strategies
  - error_reporting
related_files:
  - ../implementations/v1/issues.md
  - ../implementations/v3/development-plan.md
```

## Overview

This document provides a comprehensive explanation of error handling approaches in the Accumulate network healing process. It categorizes different types of errors, discusses strategies for handling them, and provides best practices for robust error handling across all implementation versions.

## Error Taxonomy

Errors in the Accumulate network healing process can be categorized into several types:

### 1. Network Errors

Network errors occur when there is a problem with the network connection or when a node is unreachable.

**Common Types**:
- Connection refused
- Connection timeout
- DNS resolution failure
- TLS handshake failure

**Example**:
```go
err := client.Query(ctx, url, params)
if err != nil {
    if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
        // Handle timeout error
        return fmt.Errorf("network timeout: %w", err)
    }
    // Handle other network errors
    return fmt.Errorf("network error: %w", err)
}
```

### 2. API Errors

API errors occur when the API returns an error response, indicating a problem with the request or the server.

**Common Types**:
- Invalid request parameters
- Authentication failure
- Authorization failure
- Rate limiting
- Server-side errors

**Example**:
```go
resp, err := client.Query(ctx, url, params)
if err != nil {
    if apiErr, ok := err.(*api.Error); ok {
        switch apiErr.Code {
        case api.ErrRateLimited:
            // Handle rate limiting
            return fmt.Errorf("rate limited: %w", err)
        case api.ErrInvalidParams:
            // Handle invalid parameters
            return fmt.Errorf("invalid parameters: %w", err)
        default:
            // Handle other API errors
            return fmt.Errorf("API error: %w", err)
        }
    }
    // Handle non-API errors
    return fmt.Errorf("error: %w", err)
}
```

### 3. Data Errors

Data errors occur when the data received from the API is invalid, incomplete, or inconsistent.

**Common Types**:
- Missing required fields
- Invalid field values
- Inconsistent data
- Unexpected data format

**Example**:
```go
resp, err := client.Query(ctx, url, params)
if err != nil {
    return fmt.Errorf("query error: %w", err)
}

if resp.Data == nil {
    return fmt.Errorf("missing data in response")
}

if resp.Data.Account == nil {
    return fmt.Errorf("missing account data in response")
}

if resp.Data.Account.URL == nil {
    return fmt.Errorf("missing URL in account data")
}
```

### 4. Logic Errors

Logic errors occur when there is a problem with the application logic, such as incorrect assumptions or invalid state.

**Common Types**:
- Invalid state transitions
- Incorrect assumptions
- Unhandled edge cases
- Race conditions

**Example**:
```go
func (h *Healer) HealAnchor(ctx context.Context, srcPartition, dstPartition string) error {
    if !IsValidPartition(srcPartition) {
        return fmt.Errorf("invalid source partition: %s", srcPartition)
    }
    
    if !IsValidPartition(dstPartition) {
        return fmt.Errorf("invalid destination partition: %s", dstPartition)
    }
    
    // Check for invalid state
    if h.client == nil {
        return fmt.Errorf("healer not initialized")
    }
    
    // Continue with healing logic
    // ...
}
```

## Error Handling Strategies

### 1. Retry Mechanisms

Retry mechanisms help handle transient errors by automatically retrying failed operations.

**Simple Retry**:
```go
func QueryWithRetry(ctx context.Context, client *api.Client, url *url.URL, params map[string]interface{}, maxRetries int) (interface{}, error) {
    var lastErr error
    
    for i := 0; i < maxRetries; i++ {
        resp, err := client.Query(ctx, url, params)
        if err == nil {
            return resp, nil
        }
        
        lastErr = err
        
        // Check if the error is retryable
        if !isRetryableError(err) {
            return nil, fmt.Errorf("non-retryable error: %w", err)
        }
        
        // Wait before retrying
        time.Sleep(time.Duration(i*100) * time.Millisecond)
    }
    
    return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

func isRetryableError(err error) bool {
    // Check if the error is a network error
    if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
        return true
    }
    
    // Check if the error is a rate limiting error
    if apiErr, ok := err.(*api.Error); ok && apiErr.Code == api.ErrRateLimited {
        return true
    }
    
    // Add other retryable error checks here
    
    return false
}
```

**Exponential Backoff**:
```go
func QueryWithExponentialBackoff(ctx context.Context, client *api.Client, url *url.URL, params map[string]interface{}, maxRetries int) (interface{}, error) {
    var lastErr error
    
    for i := 0; i < maxRetries; i++ {
        resp, err := client.Query(ctx, url, params)
        if err == nil {
            return resp, nil
        }
        
        lastErr = err
        
        // Check if the error is retryable
        if !isRetryableError(err) {
            return nil, fmt.Errorf("non-retryable error: %w", err)
        }
        
        // Calculate backoff duration
        backoff := time.Duration(math.Pow(2, float64(i))) * 100 * time.Millisecond
        
        // Add jitter to avoid thundering herd
        jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
        backoff = backoff + jitter
        
        // Wait before retrying
        time.Sleep(backoff)
    }
    
    return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}
```

### 2. Fallback Mechanisms

Fallback mechanisms help handle errors by providing alternative paths when the primary path fails.

**Node Fallback**:
```go
func QueryWithNodeFallback(ctx context.Context, clients []*api.Client, url *url.URL, params map[string]interface{}) (interface{}, error) {
    var lastErr error
    
    for _, client := range clients {
        resp, err := client.Query(ctx, url, params)
        if err == nil {
            return resp, nil
        }
        
        lastErr = err
        
        // Log the error and continue with the next client
        log.Printf("Error querying node %s: %v", client.Endpoint(), err)
    }
    
    return nil, fmt.Errorf("all nodes failed: %w", lastErr)
}
```

**API Version Fallback**:
```go
func QueryWithAPIVersionFallback(ctx context.Context, client *api.Client, url *url.URL, params map[string]interface{}) (interface{}, error) {
    // Try with API v3
    resp, err := client.QueryV3(ctx, url, params)
    if err == nil {
        return resp, nil
    }
    
    // Log the error and try with API v2
    log.Printf("Error querying with API v3: %v", err)
    
    resp, err = client.QueryV2(ctx, url, params)
    if err == nil {
        return resp, nil
    }
    
    // Log the error and try with API v1
    log.Printf("Error querying with API v2: %v", err)
    
    resp, err = client.QueryV1(ctx, url, params)
    if err == nil {
        return resp, nil
    }
    
    return nil, fmt.Errorf("all API versions failed: %w", err)
}
```

### 3. Circuit Breakers

Circuit breakers help prevent cascading failures by temporarily disabling operations that are consistently failing.

```go
type CircuitBreaker struct {
    failures     int
    threshold    int
    resetTimeout time.Duration
    lastFailure  time.Time
    state        string // "closed", "open", or "half-open"
    mutex        sync.RWMutex
}

func NewCircuitBreaker(threshold int, resetTimeout time.Duration) *CircuitBreaker {
    return &CircuitBreaker{
        threshold:    threshold,
        resetTimeout: resetTimeout,
        state:        "closed",
    }
}

func (cb *CircuitBreaker) Execute(fn func() error) error {
    cb.mutex.RLock()
    state := cb.state
    cb.mutex.RUnlock()
    
    if state == "open" {
        // Check if it's time to try again
        cb.mutex.RLock()
        timeSinceLastFailure := time.Since(cb.lastFailure)
        cb.mutex.RUnlock()
        
        if timeSinceLastFailure < cb.resetTimeout {
            return fmt.Errorf("circuit breaker is open")
        }
        
        // Transition to half-open state
        cb.mutex.Lock()
        cb.state = "half-open"
        cb.mutex.Unlock()
    }
    
    // Execute the function
    err := fn()
    
    if err != nil {
        cb.mutex.Lock()
        defer cb.mutex.Unlock()
        
        cb.failures++
        cb.lastFailure = time.Now()
        
        if cb.state == "half-open" || cb.failures >= cb.threshold {
            cb.state = "open"
        }
        
        return err
    }
    
    // Reset on success
    cb.mutex.Lock()
    defer cb.mutex.Unlock()
    
    cb.failures = 0
    cb.state = "closed"
    
    return nil
}
```

### 4. Error Wrapping and Unwrapping

Error wrapping and unwrapping help provide context and preserve the original error information.

```go
func QueryAccount(ctx context.Context, client *api.Client, url *url.URL) (*api.Account, error) {
    resp, err := client.Query(ctx, url, map[string]interface{}{
        "type": "account",
    })
    if err != nil {
        return nil, fmt.Errorf("failed to query account %s: %w", url, err)
    }
    
    account, ok := resp.(*api.Account)
    if !ok {
        return nil, fmt.Errorf("unexpected response type: %T", resp)
    }
    
    return account, nil
}

// Elsewhere in the code
account, err := QueryAccount(ctx, client, url)
if err != nil {
    // Check if the error is a specific type
    var apiErr *api.Error
    if errors.As(err, &apiErr) && apiErr.Code == api.ErrNotFound {
        // Handle not found error
        return fmt.Errorf("account not found: %w", err)
    }
    
    // Handle other errors
    return fmt.Errorf("error querying account: %w", err)
}
```

## Error Reporting

Effective error reporting helps diagnose and fix problems by providing detailed information about errors.

### 1. Structured Logging

Structured logging helps provide context and make errors more searchable.

```go
func HealAnchor(ctx context.Context, logger *log.Logger, srcPartition, dstPartition string) error {
    logger.Info("Starting anchor healing",
        "src_partition", srcPartition,
        "dst_partition", dstPartition,
    )
    
    // Healing logic
    // ...
    
    if err != nil {
        logger.Error("Anchor healing failed",
            "src_partition", srcPartition,
            "dst_partition", dstPartition,
            "error", err,
        )
        return err
    }
    
    logger.Info("Anchor healing completed",
        "src_partition", srcPartition,
        "dst_partition", dstPartition,
    )
    
    return nil
}
```

### 2. Error Context

Error context helps provide additional information about the error.

```go
type ErrorContext struct {
    Operation    string
    URL          *url.URL
    Partition    string
    ErrorType    string
    RetryCount   int
    ResponseCode int
}

func (ec *ErrorContext) String() string {
    return fmt.Sprintf("operation=%s url=%s partition=%s error_type=%s retry_count=%d response_code=%d",
        ec.Operation, ec.URL, ec.Partition, ec.ErrorType, ec.RetryCount, ec.ResponseCode)
}

func QueryWithContext(ctx context.Context, client *api.Client, url *url.URL, params map[string]interface{}) (interface{}, error) {
    ec := &ErrorContext{
        Operation: "query",
        URL:       url,
    }
    
    resp, err := client.Query(ctx, url, params)
    if err != nil {
        if apiErr, ok := err.(*api.Error); ok {
            ec.ErrorType = "api"
            ec.ResponseCode = apiErr.Code
        } else if netErr, ok := err.(net.Error); ok {
            ec.ErrorType = "network"
        } else {
            ec.ErrorType = "unknown"
        }
        
        return nil, fmt.Errorf("query failed: %s: %w", ec, err)
    }
    
    return resp, nil
}
```

## Implementation Comparison

| Feature | V1 | V2 | V3 | V4 |
|---------|----|----|----|----|
| Basic Error Handling | ✅ | ✅ | ✅ | ✅ |
| Retry Mechanisms | ❌ | ✅ | ✅ | ✅ |
| Fallback Mechanisms | ❌ | ✅ | ✅ | ✅ |
| Circuit Breakers | ❌ | ❌ | ✅ | ✅ |
| Error Wrapping | ❌ | ✅ | ✅ | ✅ |
| Structured Logging | ❌ | ❌ | ✅ | ✅ |
| Error Context | ❌ | ❌ | ✅ | ✅ |
| Comprehensive Error Taxonomy | ❌ | ❌ | ❌ | ✅ |

## Best Practices

1. **Use Error Wrapping**: Always wrap errors with context to provide more information about the error.
2. **Implement Retry Mechanisms**: Use retry mechanisms with exponential backoff for transient errors.
3. **Implement Fallback Mechanisms**: Use fallback mechanisms for critical operations.
4. **Use Circuit Breakers**: Use circuit breakers to prevent cascading failures.
5. **Provide Error Context**: Include context information in error messages to help diagnose problems.
6. **Use Structured Logging**: Use structured logging to make errors more searchable.
7. **Handle Different Error Types**: Handle different error types differently based on their characteristics.
8. **Fail Fast**: Fail fast for non-retryable errors to avoid wasting resources.
9. **Graceful Degradation**: Implement graceful degradation for non-critical features.
10. **Monitor and Alert**: Monitor error rates and alert on unusual patterns.

## Conclusion

Effective error handling is critical for the reliability and robustness of the Accumulate network healing process. The error handling approach should be chosen based on the specific requirements of the implementation, with consideration for the types of errors that can occur, the impact of failures, and the available recovery mechanisms.

The V4 implementation provides the most comprehensive error handling approach, with support for retry mechanisms, fallback mechanisms, circuit breakers, error wrapping, structured logging, error context, and a comprehensive error taxonomy. However, even simpler implementations can benefit from adopting some of these best practices.

## See Also

- [Healing Process Overview](./healing-process.md)
- [URL Construction](./url-construction.md)
- [Caching Strategies](./caching-strategies.md)
- [Original Issues (v1)](../implementations/v1/issues.md)
- [Enhanced Error Handling (v3)](../implementations/v3/development-plan.md)
