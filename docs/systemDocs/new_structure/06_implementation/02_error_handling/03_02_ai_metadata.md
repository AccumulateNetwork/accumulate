# AI-METADATA
document_type: implementation_detail
project: accumulate_network
component: error_handling
version: v3
related_files:
  - ../../concepts/error-handling.md
  - ../v1/issues.md
  - ../v2/error-handling.md
  - ./stateless-architecture.md
```

## Overview

This document details the error handling approach implemented in the V3 version of the Accumulate Network debug tools. The V3 implementation significantly improves upon previous versions by introducing a comprehensive, structured error handling system that aligns with the stateless architecture.

## Key Principles

The V3 error handling system is built on several key principles:

1. **Error Categorization**: Errors are categorized by type and severity
2. **Context Preservation**: Errors maintain their context throughout the call stack
3. **Structured Logging**: Errors are logged in a structured format with consistent metadata
4. **Graceful Degradation**: The system degrades gracefully when errors occur
5. **Error Recovery**: The system can recover from certain types of errors

## Error Type System

The V3 implementation introduces a strong error type system:

```go
// ErrorType represents the category of an error
type ErrorType int

const (
    // Error categories
    ErrorTypeUnknown ErrorType = iota
    ErrorTypeNetwork           // Network-related errors
    ErrorTypeRouting           // Routing-related errors
    ErrorTypeValidation        // Validation-related errors
    ErrorTypeState             // State-related errors
    ErrorTypeConsensus         // Consensus-related errors
    ErrorTypeInternal          // Internal errors
)

// ErrorSeverity represents the severity of an error
type ErrorSeverity int

const (
    // Error severities
    SeverityDebug    ErrorSeverity = iota // Debugging information
    SeverityInfo                          // Informational messages
    SeverityWarning                       // Warning conditions
    SeverityError                         // Error conditions
    SeverityCritical                      // Critical conditions
    SeverityFatal                         // Fatal conditions
)

// AccError represents a structured error in the Accumulate system
type AccError struct {
    Type        ErrorType     // Error category
    Severity    ErrorSeverity // Error severity
    Code        string        // Error code
    Message     string        // Human-readable error message
    Details     interface{}   // Additional error details
    Cause       error         // Underlying cause
    StackTrace  string        // Stack trace
    Recoverable bool          // Whether the error is recoverable
    Timestamp   time.Time     // When the error occurred
}

// Error implements the error interface
func (e *AccError) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("%s: %v", e.Message, e.Cause)
    }
    return e.Message
}

// Unwrap implements the errors.Unwrap interface
func (e *AccError) Unwrap() error {
    return e.Cause
}
```

## Error Creation

The V3 implementation provides factory functions for creating errors:

```go
// NewError creates a new AccError
func NewError(errType ErrorType, severity ErrorSeverity, code string, message string, cause error) *AccError {
    return &AccError{
        Type:        errType,
        Severity:    severity,
        Code:        code,
        Message:     message,
        Cause:       cause,
        StackTrace:  captureStackTrace(),
        Recoverable: isRecoverable(errType, severity),
        Timestamp:   time.Now(),
    }
}

// NewNetworkError creates a new network error
func NewNetworkError(code string, message string, cause error) *AccError {
    return NewError(ErrorTypeNetwork, SeverityError, code, message, cause)
}

// NewRoutingError creates a new routing error
func NewRoutingError(code string, message string, cause error) *AccError {
    return NewError(ErrorTypeRouting, SeverityError, code, message, cause)
}

// NewValidationError creates a new validation error
func NewValidationError(code string, message string, cause error) *AccError {
    return NewError(ErrorTypeValidation, SeverityError, code, message, cause)
}

// NewStateError creates a new state error
func NewStateError(code string, message string, cause error) *AccError {
    return NewError(ErrorTypeState, SeverityError, code, message, cause)
}

// NewConsensusError creates a new consensus error
func NewConsensusError(code string, message string, cause error) *AccError {
    return NewError(ErrorTypeConsensus, SeverityError, code, message, cause)
}

// NewInternalError creates a new internal error
func NewInternalError(code string, message string, cause error) *AccError {
    return NewError(ErrorTypeInternal, SeverityCritical, code, message, cause)
}
```

## Error Handling Patterns

### 1. Error Wrapping

The V3 implementation uses error wrapping to preserve context:

```go
// WrapError wraps an error with additional context
func WrapError(err error, message string) error {
    if err == nil {
        return nil
    }
    
    // If it's already an AccError, wrap it while preserving its type
    if accErr, ok := err.(*AccError); ok {
        return &AccError{
            Type:        accErr.Type,
            Severity:    accErr.Severity,
            Code:        accErr.Code,
            Message:     message + ": " + accErr.Message,
            Details:     accErr.Details,
            Cause:       accErr.Cause,
            StackTrace:  captureStackTrace(),
            Recoverable: accErr.Recoverable,
            Timestamp:   time.Now(),
        }
    }
    
    // Otherwise, create a new unknown error
    return NewError(ErrorTypeUnknown, SeverityError, "UNKNOWN", message, err)
}
```

### 2. Error Checking

The V3 implementation provides utilities for checking error types:

```go
// IsNetworkError checks if an error is a network error
func IsNetworkError(err error) bool {
    var accErr *AccError
    if errors.As(err, &accErr) {
        return accErr.Type == ErrorTypeNetwork
    }
    return false
}

// IsRoutingError checks if an error is a routing error
func IsRoutingError(err error) bool {
    var accErr *AccError
    if errors.As(err, &accErr) {
        return accErr.Type == ErrorTypeRouting
    }
    return false
}

// IsValidationError checks if an error is a validation error
func IsValidationError(err error) bool {
    var accErr *AccError
    if errors.As(err, &accErr) {
        return accErr.Type == ErrorTypeValidation
    }
    return false
}

// IsRecoverable checks if an error is recoverable
func IsRecoverable(err error) bool {
    var accErr *AccError
    if errors.As(err, &accErr) {
        return accErr.Recoverable
    }
    return false
}
```

### 3. Error Recovery

The V3 implementation includes mechanisms for recovering from errors:

```go
// RecoverFromPanic recovers from a panic and returns it as an error
func RecoverFromPanic() error {
    if r := recover(); r != nil {
        switch v := r.(type) {
        case error:
            return NewInternalError("PANIC", "Panic recovered", v)
        default:
            return NewInternalError("PANIC", fmt.Sprintf("Panic recovered: %v", r), nil)
        }
    }
    return nil
}

// WithRecovery executes a function with panic recovery
func WithRecovery(fn func() error) (err error) {
    defer func() {
        if r := recover(); r != nil {
            switch v := r.(type) {
            case error:
                err = NewInternalError("PANIC", "Panic recovered", v)
            default:
                err = NewInternalError("PANIC", fmt.Sprintf("Panic recovered: %v", r), nil)
            }
        }
    }()
    
    return fn()
}
```

## Error Logging

The V3 implementation includes structured error logging:

```go
// LogError logs an error with structured metadata
func LogError(ctx context.Context, err error) {
    if err == nil {
        return
    }
    
    // Extract AccError if possible
    var accErr *AccError
    if errors.As(err, &accErr) {
        // Log with structured metadata
        logger := log.WithFields(log.Fields{
            "error_type":     accErr.Type.String(),
            "error_severity": accErr.Severity.String(),
            "error_code":     accErr.Code,
            "recoverable":    accErr.Recoverable,
            "timestamp":      accErr.Timestamp,
        })
        
        // Add context values if available
        if ctx != nil {
            if reqID, ok := ctx.Value("request_id").(string); ok {
                logger = logger.WithField("request_id", reqID)
            }
            if userID, ok := ctx.Value("user_id").(string); ok {
                logger = logger.WithField("user_id", userID)
            }
        }
        
        // Log at appropriate level
        switch accErr.Severity {
        case SeverityDebug:
            logger.Debug(accErr.Message)
        case SeverityInfo:
            logger.Info(accErr.Message)
        case SeverityWarning:
            logger.Warn(accErr.Message)
        case SeverityError:
            logger.Error(accErr.Message)
        case SeverityCritical, SeverityFatal:
            logger.Fatal(accErr.Message)
        }
        
        // Log stack trace at debug level
        if accErr.StackTrace != "" {
            log.Debugf("Stack trace: %s", accErr.StackTrace)
        }
    } else {
        // Log regular error
        log.Error(err)
    }
}
```

## Integration with Stateless Architecture

The error handling system is integrated with the V3 stateless architecture:

### 1. Context-Aware Error Handling

```go
// ProcessRequest processes a request with context-aware error handling
func (h *Handler) ProcessRequest(ctx context.Context, req *Request) (*Response, error) {
    // Create a context with request ID
    reqID := generateRequestID()
    ctx = context.WithValue(ctx, "request_id", reqID)
    
    // Process the request with recovery
    resp, err := WithRecovery(func() (*Response, error) {
        return h.processRequestInternal(ctx, req)
    })
    
    // Handle errors
    if err != nil {
        LogError(ctx, err)
        
        // Check if error is recoverable
        if IsRecoverable(err) {
            // Try to recover
            recoveryResp, recoveryErr := h.recoverFromError(ctx, req, err)
            if recoveryErr == nil {
                log.WithField("request_id", reqID).Info("Successfully recovered from error")
                return recoveryResp, nil
            }
            
            // Recovery failed
            LogError(ctx, WrapError(recoveryErr, "Recovery failed"))
        }
        
        // Return appropriate error response
        return h.createErrorResponse(ctx, req, err), nil
    }
    
    return resp, nil
}
```

### 2. Error-Aware Routing

```go
// RouteRequest routes a request with error handling
func (r *Router) RouteRequest(ctx context.Context, req *Request) (*Response, error) {
    // Parse and validate the URL
    url, err := ParseAccURL(req.URL)
    if err != nil {
        return nil, NewValidationError("INVALID_URL", "Invalid URL", err)
    }
    
    // Determine the target partition
    partition, err := url.Partition()
    if err != nil {
        return nil, NewRoutingError("PARTITION_ERROR", "Could not determine target partition", err)
    }
    
    // Get nodes for the partition
    nodes, err := r.nodeRegistry.GetNodesForPartition(partition)
    if err != nil {
        return nil, NewRoutingError("NO_NODES", fmt.Sprintf("No nodes found for partition %s", partition), err)
    }
    
    // Select the best node
    node, err := r.nodeSelector.SelectNode(nodes)
    if err != nil {
        return nil, NewRoutingError("NODE_SELECTION", "Failed to select node", err)
    }
    
    // Forward the request to the selected node
    resp, err := r.forwardRequest(ctx, node, req)
    if err != nil {
        // Check if it's a network error
        if IsNetworkError(err) {
            // Try another node
            alternativeNode, altErr := r.nodeSelector.SelectAlternativeNode(nodes, node)
            if altErr == nil {
                log.WithContext(ctx).Infof("Retrying request on alternative node %s", alternativeNode.ID)
                return r.forwardRequest(ctx, alternativeNode, req)
            }
        }
        
        return nil, err
    }
    
    return resp, nil
}
```

### 3. Error-Aware Caching

```go
// Get retrieves a value from the cache with error handling
func (c *Cache) Get(ctx context.Context, key string) (interface{}, error) {
    // Check if key is valid
    if key == "" {
        return nil, NewValidationError("INVALID_KEY", "Cache key cannot be empty", nil)
    }
    
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    // Check if key exists
    entry, found := c.cache[key]
    if !found {
        return nil, NewStateError("KEY_NOT_FOUND", fmt.Sprintf("Cache key not found: %s", key), nil)
    }
    
    // Check if entry has expired
    if time.Now().After(entry.expiresAt) {
        return nil, NewStateError("ENTRY_EXPIRED", fmt.Sprintf("Cache entry expired: %s", key), nil)
    }
    
    return entry.data, nil
}
```

## Error Codes

The V3 implementation includes a comprehensive set of error codes:

### Network Error Codes

- `CONN_REFUSED`: Connection refused
- `TIMEOUT`: Connection timeout
- `DNS_FAILURE`: DNS resolution failure
- `TLS_ERROR`: TLS/SSL error
- `CONN_RESET`: Connection reset
- `CONN_CLOSED`: Connection closed

### Routing Error Codes

- `INVALID_PARTITION`: Invalid partition
- `NO_NODES`: No nodes available for partition
- `NODE_SELECTION`: Node selection failure
- `ROUTE_NOT_FOUND`: Route not found
- `CONFLICTING_ROUTES`: Conflicting routes

### Validation Error Codes

- `INVALID_URL`: Invalid URL
- `INVALID_PARAMETER`: Invalid parameter
- `MISSING_PARAMETER`: Missing parameter
- `INVALID_FORMAT`: Invalid format
- `INVALID_SIGNATURE`: Invalid signature
- `INVALID_SEQUENCE`: Invalid sequence number

### State Error Codes

- `ACCOUNT_NOT_FOUND`: Account not found
- `CHAIN_NOT_FOUND`: Chain not found
- `TRANSACTION_NOT_FOUND`: Transaction not found
- `INVALID_STATE`: Invalid state
- `STATE_MISMATCH`: State mismatch
- `SEQUENCE_GAP`: Sequence gap

### Consensus Error Codes

- `CONSENSUS_FAILURE`: Consensus failure
- `VALIDATION_FAILURE`: Validation failure
- `ANCHOR_FAILURE`: Anchor failure
- `SYNTHETIC_FAILURE`: Synthetic transaction failure
- `RECEIPT_FAILURE`: Receipt failure

### Internal Error Codes

- `PANIC`: Panic recovered
- `ASSERTION_FAILURE`: Assertion failure
- `INTERNAL_ERROR`: Internal error
- `UNEXPECTED_ERROR`: Unexpected error
- `CONFIGURATION_ERROR`: Configuration error

## Error Recovery Strategies

The V3 implementation includes several error recovery strategies:

### 1. Node Failover

```go
// tryWithFailover tries an operation with node failover
func tryWithFailover(ctx context.Context, nodes []Node, op func(Node) error) error {
    if len(nodes) == 0 {
        return NewRoutingError("NO_NODES", "No nodes available", nil)
    }
    
    // Try primary node
    err := op(nodes[0])
    if err == nil {
        return nil
    }
    
    // If it's a network error, try alternative nodes
    if IsNetworkError(err) {
        for i := 1; i < len(nodes); i++ {
            altErr := op(nodes[i])
            if altErr == nil {
                return nil
            }
            
            // Log alternative node failure
            LogError(ctx, WrapError(altErr, fmt.Sprintf("Alternative node %s failed", nodes[i].ID)))
        }
    }
    
    return err
}
```

### 2. Retry with Backoff

```go
// retryWithBackoff retries an operation with exponential backoff
func retryWithBackoff(ctx context.Context, maxRetries int, initialDelay time.Duration, op func() error) error {
    var err error
    delay := initialDelay
    
    for i := 0; i < maxRetries; i++ {
        // Check if context is canceled
        if ctx.Err() != nil {
            return WrapError(ctx.Err(), "Context canceled during retry")
        }
        
        // Try the operation
        err = op()
        if err == nil {
            return nil
        }
        
        // If it's not a recoverable error, don't retry
        if !IsRecoverable(err) {
            return err
        }
        
        // Log retry attempt
        log.WithContext(ctx).Infof("Retry %d/%d failed: %v (retrying in %v)", i+1, maxRetries, err, delay)
        
        // Wait before retrying
        select {
        case <-time.After(delay):
            // Increase delay for next retry (exponential backoff)
            delay *= 2
        case <-ctx.Done():
            return WrapError(ctx.Err(), "Context canceled during retry delay")
        }
    }
    
    return WrapError(err, fmt.Sprintf("Failed after %d retries", maxRetries))
}
```

### 3. Circuit Breaker

```go
// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
    name           string
    maxFailures    int
    resetTimeout   time.Duration
    failureCount   int
    lastFailure    time.Time
    state          CircuitState
    mutex          sync.RWMutex
}

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
    CircuitClosed CircuitState = iota // Normal operation
    CircuitOpen                       // Failing, not allowing requests
    CircuitHalfOpen                   // Testing if service is healthy
)

// Execute executes an operation with circuit breaker protection
func (cb *CircuitBreaker) Execute(ctx context.Context, op func() error) error {
    cb.mutex.RLock()
    state := cb.state
    cb.mutex.RUnlock()
    
    // Check if circuit is open
    if state == CircuitOpen {
        // Check if reset timeout has elapsed
        cb.mutex.RLock()
        elapsed := time.Since(cb.lastFailure)
        cb.mutex.RUnlock()
        
        if elapsed < cb.resetTimeout {
            return NewInternalError("CIRCUIT_OPEN", fmt.Sprintf("Circuit %s is open", cb.name), nil)
        }
        
        // Reset timeout has elapsed, transition to half-open
        cb.mutex.Lock()
        cb.state = CircuitHalfOpen
        cb.mutex.Unlock()
    }
    
    // Execute the operation
    err := op()
    
    // Update circuit state based on result
    cb.mutex.Lock()
    defer cb.mutex.Unlock()
    
    if err != nil {
        // Operation failed
        cb.failureCount++
        cb.lastFailure = time.Now()
        
        // Check if we should open the circuit
        if cb.state == CircuitHalfOpen || cb.failureCount >= cb.maxFailures {
            cb.state = CircuitOpen
            log.WithContext(ctx).Warnf("Circuit %s is now open due to failures", cb.name)
        }
    } else {
        // Operation succeeded
        if cb.state == CircuitHalfOpen {
            // Success in half-open state, close the circuit
            cb.state = CircuitClosed
            cb.failureCount = 0
            log.WithContext(ctx).Infof("Circuit %s is now closed", cb.name)
        } else if cb.state == CircuitClosed {
            // Success in closed state, reset failure count
            cb.failureCount = 0
        }
    }
    
    return err
}
```

## Error Monitoring and Metrics

The V3 implementation includes comprehensive error monitoring:

```go
// ErrorMetrics tracks error-related metrics
type ErrorMetrics struct {
    // Error counts by type
    NetworkErrors   prometheus.Counter
    RoutingErrors   prometheus.Counter
    ValidationErrors prometheus.Counter
    StateErrors     prometheus.Counter
    ConsensusErrors prometheus.Counter
    InternalErrors  prometheus.Counter
    
    // Error counts by severity
    WarningErrors   prometheus.Counter
    ErrorErrors     prometheus.Counter
    CriticalErrors  prometheus.Counter
    FatalErrors     prometheus.Counter
    
    // Recovery metrics
    RecoveryAttempts prometheus.Counter
    RecoverySuccesses prometheus.Counter
    RecoveryFailures prometheus.Counter
    
    // Circuit breaker metrics
    CircuitOpenEvents prometheus.Counter
    CircuitCloseEvents prometheus.Counter
}

// NewErrorMetrics creates a new error metrics tracker
func NewErrorMetrics(registry prometheus.Registerer) *ErrorMetrics {
    metrics := &ErrorMetrics{
        // Initialize counters
        // ...
    }
    
    // Register metrics with Prometheus
    registry.MustRegister(
        metrics.NetworkErrors,
        metrics.RoutingErrors,
        // ...
    )
    
    return metrics
}

// RecordError records an error in the metrics
func (m *ErrorMetrics) RecordError(err error) {
    if err == nil {
        return
    }
    
    // Extract AccError if possible
    var accErr *AccError
    if errors.As(err, &accErr) {
        // Increment type counter
        switch accErr.Type {
        case ErrorTypeNetwork:
            m.NetworkErrors.Inc()
        case ErrorTypeRouting:
            m.RoutingErrors.Inc()
        case ErrorTypeValidation:
            m.ValidationErrors.Inc()
        case ErrorTypeState:
            m.StateErrors.Inc()
        case ErrorTypeConsensus:
            m.ConsensusErrors.Inc()
        case ErrorTypeInternal:
            m.InternalErrors.Inc()
        }
        
        // Increment severity counter
        switch accErr.Severity {
        case SeverityWarning:
            m.WarningErrors.Inc()
        case SeverityError:
            m.ErrorErrors.Inc()
        case SeverityCritical:
            m.CriticalErrors.Inc()
        case SeverityFatal:
            m.FatalErrors.Inc()
        }
    } else {
        // Unknown error type
        m.InternalErrors.Inc()
        m.ErrorErrors.Inc()
    }
}
```

## Comparison with Previous Versions

| Aspect | V1 | V2 | V3 |
|--------|----|----|-----|
| Error Types | Generic errors | Basic error categories | Comprehensive type system |
| Error Context | Limited or none | Basic context in some errors | Rich context throughout |
| Error Recovery | Manual, ad-hoc | Basic retry mechanisms | Structured recovery strategies |
| Logging | Inconsistent | Improved but still inconsistent | Structured, consistent logging |
| Metrics | None | Basic error counting | Comprehensive error metrics |
| Circuit Breaking | None | Limited | Full circuit breaker implementation |

## Best Practices

When working with the V3 error handling system, follow these best practices:

1. **Use Typed Errors**: Use the provided error types and factory functions
2. **Preserve Context**: Wrap errors with additional context using `WrapError`
3. **Check Error Types**: Use the provided type checking functions
4. **Handle Recovery**: Use `WithRecovery` for critical operations
5. **Log Properly**: Use `LogError` for consistent logging
6. **Monitor Metrics**: Track error metrics for operational insights

## Conclusion

The error handling system in the V3 implementation provides a comprehensive, structured approach to managing errors in the Accumulate Network debug tools. By categorizing errors, preserving context, implementing recovery strategies, and providing consistent logging and metrics, it significantly improves the reliability and observability of the system.

The integration with the stateless architecture ensures that error handling is consistent across all components of the system, from routing to caching to transaction processing. The comprehensive set of error codes and recovery strategies enables more precise error handling and better user experiences.

## See Also

- [Error Handling Approaches](../../concepts/error-handling.md): Core concepts of error handling
- [Known Issues in V1](../v1/issues.md): Error handling issues in the V1 implementation
- [Error Handling in V2](../v2/error-handling.md): Error handling in the V2 implementation
- [Stateless Architecture](./stateless-architecture.md): Overview of the V3 stateless architecture
