# Phase 2: Enhanced Client

## Objective
Add options pattern to the client for flexible configuration, implement circuit breaker pattern, and provide better defaults.

## Timeline
Week 2 (5 days)

## Dependencies
- Phase 1 must be complete (connection pooling and retry logic)

## Components

### 1. Client Options Pattern (`/pkg/client/options.go`)

Add flexible configuration options:

```go
package client

type ClientOption func(*Client)

// WithConnectionPool enables connection pooling
func WithConnectionPool() ClientOption {
    return func(c *Client) {
        jrpcClient := GetPooledClient(c.config.Endpoint)
        jrpcClient.Debug = c.config.Debug
        
        c.v3Client = jrpcClient
        c.nodeService = jrpcClient
        c.networkService = jrpcClient
        c.submitter = jrpcClient
        c.validator = jrpcClient
        c.faucet = jrpcClient
    }
}

// WithRetry enables retry logic
func WithRetry(cfg RetryConfig) ClientOption {
    return func(c *Client) {
        c.retryConfig = &cfg
    }
}

// WithTimeout sets custom timeout
func WithTimeout(timeout time.Duration) ClientOption {
    return func(c *Client) {
        c.config.Timeout = timeout
    }
}

// WithCircuitBreaker enables circuit breaker
func WithCircuitBreaker(threshold float64, timeout time.Duration) ClientOption {
    return func(c *Client) {
        c.circuitBreaker = &CircuitBreaker{
            FailureThreshold: threshold,
            ResetTimeout:    timeout,
        }
    }
}
```

### 2. Circuit Breaker (`/pkg/client/circuit_breaker.go`)

Implement circuit breaker pattern:

```go
package client

type CircuitBreaker struct {
    FailureThreshold float64
    ResetTimeout     time.Duration
    
    mu            sync.RWMutex
    failures      int64
    successes     int64
    lastFailure   time.Time
    state         CircuitState
}

type CircuitState int

const (
    CircuitClosed CircuitState = iota
    CircuitOpen
    CircuitHalfOpen
)

func (cb *CircuitBreaker) Call(fn func() error) error {
    cb.mu.RLock()
    state := cb.state
    cb.mu.RUnlock()
    
    if state == CircuitOpen {
        if time.Since(cb.lastFailure) > cb.ResetTimeout {
            cb.mu.Lock()
            cb.state = CircuitHalfOpen
            cb.mu.Unlock()
        } else {
            return errors.New("circuit breaker open")
        }
    }
    
    err := fn()
    
    cb.mu.Lock()
    defer cb.mu.Unlock()
    
    if err != nil {
        cb.failures++
        cb.lastFailure = time.Now()
        
        if float64(cb.failures)/(float64(cb.failures+cb.successes)) > cb.FailureThreshold {
            cb.state = CircuitOpen
        }
        return err
    }
    
    cb.successes++
    if cb.state == CircuitHalfOpen {
        cb.state = CircuitClosed
        cb.failures = 0
        cb.successes = 0
    }
    
    return nil
}
```

### 3. Enhanced Client Methods (`/pkg/client/client.go` modifications)

Update client to support options:

```go
// Add to existing client.go

func NewWithOptions(config *Config, opts ...ClientOption) (*Client, error) {
    // Create base client
    client, err := New(config)
    if err != nil {
        return nil, err
    }
    
    // Apply options
    for _, opt := range opts {
        opt(client)
    }
    
    return client, nil
}

// Add retry wrapper method
func (c *Client) GetAccountWithRetry(ctx context.Context, url string) (*Account, error) {
    if c.retryConfig == nil {
        return c.GetAccount(ctx, url)
    }
    
    var result *Account
    err := WithRetry(ctx, *c.retryConfig, func(ctx context.Context) error {
        var err error
        result, err = c.GetAccount(ctx, url)
        return err
    })
    
    return result, err
}

// Add circuit breaker wrapper
func (c *Client) executeWithCircuitBreaker(fn func() error) error {
    if c.circuitBreaker == nil {
        return fn()
    }
    return c.circuitBreaker.Call(fn)
}
```

## Implementation Steps

### Day 1: Options Pattern
1. Create `/pkg/client/options.go`
2. Define ClientOption type and option functions
3. Test option application
4. Document usage patterns

### Day 2: Circuit Breaker
1. Create `/pkg/client/circuit_breaker.go`
2. Implement circuit breaker logic
3. Add state management
4. Unit test circuit breaker states

### Day 3: Client Integration
1. Modify `/pkg/client/client.go`
2. Add `NewWithOptions` constructor
3. Add retry wrapper methods
4. Integrate circuit breaker

### Day 4: Default Configurations
1. Create sensible default configurations
2. Add environment variable support
3. Create preset configurations (production, development)
4. Document configuration best practices

### Day 5: Testing & Examples
1. Update examples to use new options
2. Test circuit breaker under failure scenarios
3. Performance test with all features enabled
4. Create migration guide

## Usage Examples

### Basic Options
```go
client, err := client.NewWithOptions(
    &client.Config{
        Endpoint: "http://localhost:26660/v3",
    },
    client.WithConnectionPool(),
    client.WithRetry(client.DefaultRetryConfig),
)
```

### Production Configuration
```go
client, err := client.NewWithOptions(
    &client.Config{
        Endpoint: endpoint,
    },
    client.WithConnectionPool(),
    client.WithRetry(client.RetryConfig{
        MaxAttempts:   5,
        InitialDelay:  200 * time.Millisecond,
        MaxDelay:      10 * time.Second,
        BackoffFactor: 2.0,
    }),
    client.WithCircuitBreaker(0.5, 30*time.Second),
    client.WithTimeout(30*time.Second),
)
```

### Environment-Based Config
```go
func NewClientFromEnv() (*Client, error) {
    opts := []ClientOption{}
    
    if os.Getenv("ACCUMULATE_CONNECTION_POOL") == "true" {
        opts = append(opts, client.WithConnectionPool())
    }
    
    if os.Getenv("ACCUMULATE_ENABLE_RETRY") == "true" {
        opts = append(opts, client.WithRetry(client.DefaultRetryConfig))
    }
    
    return client.NewWithOptions(config, opts...)
}
```

## Success Metrics

- [ ] Options pattern allows flexible configuration
- [ ] Circuit breaker prevents cascade failures
- [ ] Retry wrapper methods work transparently
- [ ] No breaking changes to existing API
- [ ] All existing tests pass

## Files Created

1. `/pkg/client/options.go` - Client options implementation
2. `/pkg/client/circuit_breaker.go` - Circuit breaker pattern

## Files Modified

1. `/pkg/client/client.go` - Add NewWithOptions and wrapper methods

## Testing Checklist

- [ ] Unit tests for each option
- [ ] Circuit breaker state transitions
- [ ] Integration with Phase 1 components
- [ ] Load test with circuit breaker
- [ ] Examples updated and working
- [ ] Environment variable configuration

## Next Phase

[Phase 3: Smart Routing](phase3-smart-routing.md) - Add health monitoring and intelligent routing