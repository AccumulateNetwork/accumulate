package server

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/mcp/client"
)

// RetryConfig defines retry behavior for transient failures
type RetryConfig struct {
	MaxRetries     int           // Maximum number of retry attempts
	InitialBackoff time.Duration // Initial backoff duration
	MaxBackoff     time.Duration // Maximum backoff duration
	BackoffFactor  float64       // Multiplier for backoff on each retry
}

// DefaultRetryConfig returns sensible defaults for retry behavior
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 500 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		BackoffFactor:  2.0,
	}
}

// isRetryableError determines if an error is transient and should be retried
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Network-related transient errors
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout() || netErr.Temporary()
	}

	// Common transient error patterns
	transientPatterns := []string{
		"connection refused",
		"connection reset",
		"connection timed out",
		"no such host",
		"temporary failure",
		"too many open files",
		"i/o timeout",
		"EOF",
		"broken pipe",
		"network is unreachable",
		"no route to host",
		"transport endpoint is not connected",
		"read: connection reset by peer",
		"write: broken pipe",
		"context deadline exceeded",
	}

	for _, pattern := range transientPatterns {
		if strings.Contains(strings.ToLower(errStr), strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}

// ClientHelper provides shared functionality for tool implementations
type ClientHelper struct {
	state *State
	cfg   *Config
}

// NewClientHelper creates a new client helper
func NewClientHelper(state *State) *ClientHelper {
	return &ClientHelper{
		state: state,
		cfg:   LoadConfig(),
	}
}

// WithClient creates a client, executes the function, and ensures cleanup
func (h *ClientHelper) WithClient(network string, fn func(ctx context.Context, c *client.Client) error) error {
	if network == "" {
		network = h.state.GetNetwork()
	}

	c, err := client.NewClient(network)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), h.cfg.APITimeout)
	defer cancel()

	return fn(ctx, c)
}

// WithClientResult creates a client, executes the function, and returns the result
func (h *ClientHelper) WithClientResult(network string, fn func(ctx context.Context, c *client.Client) (map[string]interface{}, error)) (map[string]interface{}, error) {
	if network == "" {
		network = h.state.GetNetwork()
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), h.cfg.APITimeout)
	defer cancel()

	result, err := fn(ctx, c)
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("operation timed out after %v", h.cfg.APITimeout)
	}
	return result, err
}

// WithClientCustomTimeout creates a client with custom timeout
func (h *ClientHelper) WithClientCustomTimeout(network string, timeout time.Duration, fn func(ctx context.Context, c *client.Client) (map[string]interface{}, error)) (map[string]interface{}, error) {
	if network == "" {
		network = h.state.GetNetwork()
	}

	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := fn(ctx, c)
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("operation timed out after %v", timeout)
	}
	return result, err
}

// GetNetwork returns the current network from state or default
func (h *ClientHelper) GetNetwork(args map[string]interface{}) string {
	if network, ok := args["network"].(string); ok && network != "" {
		return network
	}
	return h.state.GetNetwork()
}

// WithClientRetry creates a client and executes the function with automatic retry for transient failures
func (h *ClientHelper) WithClientRetry(network string, retryConfig *RetryConfig, fn func(ctx context.Context, c *client.Client) (map[string]interface{}, error)) (map[string]interface{}, error) {
	if network == "" {
		network = h.state.GetNetwork()
	}

	if retryConfig == nil {
		retryConfig = DefaultRetryConfig()
	}

	var lastErr error
	backoff := retryConfig.InitialBackoff

	for attempt := 0; attempt <= retryConfig.MaxRetries; attempt++ {
		// Wait before retry (skip on first attempt)
		if attempt > 0 {
			time.Sleep(backoff)
			// Increase backoff for next attempt
			backoff = time.Duration(float64(backoff) * retryConfig.BackoffFactor)
			if backoff > retryConfig.MaxBackoff {
				backoff = retryConfig.MaxBackoff
			}
		}

		c, err := client.NewClient(network)
		if err != nil {
			if isRetryableError(err) {
				lastErr = err
				continue
			}
			return nil, fmt.Errorf("failed to create client: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), h.cfg.APITimeout)
		result, err := fn(ctx, c)
		cancel()
		c.Close()

		if err == nil {
			return result, nil
		}

		if ctx.Err() == context.DeadlineExceeded {
			lastErr = fmt.Errorf("operation timed out after %v", h.cfg.APITimeout)
			// Timeouts are retryable
			continue
		}

		if isRetryableError(err) {
			lastErr = err
			continue
		}

		// Non-retryable error
		return nil, err
	}

	return nil, fmt.Errorf("operation failed after %d retries: %w", retryConfig.MaxRetries, lastErr)
}

// GetStringArg gets a string argument with default value
func GetStringArg(args map[string]interface{}, key string, defaultValue string) string {
	if val, ok := args[key].(string); ok && val != "" {
		return val
	}
	return defaultValue
}

// GetIntArg gets an integer argument with default value
func GetIntArg(args map[string]interface{}, key string, defaultValue int) int {
	if val, ok := args[key].(float64); ok {
		return int(val)
	}
	return defaultValue
}

// GetBoolArg gets a boolean argument with default value
func GetBoolArg(args map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := args[key].(bool); ok {
		return val
	}
	return defaultValue
}

// RequireStringArg gets a required string argument or returns error
func RequireStringArg(args map[string]interface{}, key string) (string, error) {
	val, ok := args[key].(string)
	if !ok || val == "" {
		return "", fmt.Errorf("missing required parameter: %s", key)
	}
	return val, nil
}
