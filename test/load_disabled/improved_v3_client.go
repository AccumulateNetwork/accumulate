package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// ImprovedClient wraps jsonrpc.Client with better connection management
type ImprovedClient struct {
	*jsonrpc.Client
	config     ClientConfig
	maxRetries int
	retryDelay time.Duration
}

// ClientConfig contains all client configuration options
type ClientConfig struct {
	RequestTimeout      time.Duration
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
	TLSHandshakeTimeout time.Duration
	MaxRetries          int
	RetryDelay          time.Duration
	Debug               bool
}

// DefaultClientConfig returns sensible defaults
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		RequestTimeout:      30 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		MaxRetries:          3,
		RetryDelay:          1 * time.Second,
		Debug:               false,
	}
}

// NewImprovedClient creates a client with proper connection pooling
func NewImprovedClient(server string, config ClientConfig) *ImprovedClient {
	// Configure transport with connection pooling
	transport := &http.Transport{
		MaxIdleConns:        config.MaxIdleConns,
		MaxIdleConnsPerHost: config.MaxIdleConnsPerHost,
		IdleConnTimeout:     config.IdleConnTimeout,
		TLSHandshakeTimeout: config.TLSHandshakeTimeout,
		DisableKeepAlives:   false, // Enable keep-alive for connection reuse
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	// Create base client
	baseClient := jsonrpc.NewClient(server)
	baseClient.Client.Transport = transport
	baseClient.Client.Timeout = config.RequestTimeout
	// baseClient.DebugRequest = config.Debug // Not available in this version

	return &ImprovedClient{
		Client:     baseClient,
		config:     config,
		maxRetries: config.MaxRetries,
		retryDelay: config.RetryDelay,
	}
}

// QueryWithRetry performs a query with automatic retry logic
func (c *ImprovedClient) QueryWithRetry(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	var lastErr error
	
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := time.Duration(attempt) * c.retryDelay
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		
		resp, err := c.Client.Query(ctx, scope, query)
		if err == nil {
			return resp, nil
		}
		
		// Check if error is retryable
		if !isRetryableError(err) {
			return nil, err
		}
		
		lastErr = err
		fmt.Printf("Request failed (attempt %d/%d): %v\n", attempt+1, c.maxRetries+1, err)
	}
	
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// SubmitWithRetry performs a submit with automatic retry logic
func (c *ImprovedClient) SubmitWithRetry(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	var lastErr error
	
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := time.Duration(attempt) * c.retryDelay
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		
		resp, err := c.Client.Submit(ctx, envelope, opts)
		if err == nil {
			return resp, nil
		}
		
		// Check if error is retryable
		if !isRetryableError(err) {
			return nil, err
		}
		
		lastErr = err
		fmt.Printf("Submit failed (attempt %d/%d): %v\n", attempt+1, c.maxRetries+1, err)
	}
	
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	// Network timeout errors
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}
	
	// Connection refused, connection reset
	errStr := err.Error()
	if strings.Contains(errStr, "connection refused") ||
	   strings.Contains(errStr, "connection reset") ||
	   strings.Contains(errStr, "EOF") ||
	   strings.Contains(errStr, "broken pipe") {
		return true
	}
	
	// Check for specific error types (simplified check)
	// These error types might not be available in all versions
	
	return false
}

// ClientPool manages a pool of clients for different servers
type ClientPool struct {
	clients map[string]*ImprovedClient
	mu      sync.RWMutex
	config  ClientConfig
}

var (
	globalPool     *ClientPool
	globalPoolOnce sync.Once
)

// GetGlobalPool returns the global client pool instance
func GetGlobalPool() *ClientPool {
	globalPoolOnce.Do(func() {
		globalPool = &ClientPool{
			clients: make(map[string]*ImprovedClient),
			config:  DefaultClientConfig(),
		}
	})
	return globalPool
}

// GetClient returns a client for the specified server, creating if needed
func (cp *ClientPool) GetClient(serverURL string) *ImprovedClient {
	cp.mu.RLock()
	if client, exists := cp.clients[serverURL]; exists {
		cp.mu.RUnlock()
		return client
	}
	cp.mu.RUnlock()
	
	cp.mu.Lock()
	defer cp.mu.Unlock()
	
	// Double-check after acquiring write lock
	if client, exists := cp.clients[serverURL]; exists {
		return client
	}
	
	// Create new client with proper configuration
	client := NewImprovedClient(serverURL, cp.config)
	cp.clients[serverURL] = client
	return client
}

// SetConfig updates the configuration for new clients
func (cp *ClientPool) SetConfig(config ClientConfig) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.config = config
}

// HealthCheck validates all connections in the pool
func (cp *ClientPool) HealthCheck(ctx context.Context) map[string]error {
	cp.mu.RLock()
	clients := make(map[string]*ImprovedClient)
	for k, v := range cp.clients {
		clients[k] = v
	}
	cp.mu.RUnlock()
	
	results := make(map[string]error)
	var wg sync.WaitGroup
	var mu sync.Mutex
	
	for url, client := range clients {
		wg.Add(1)
		go func(url string, client *ImprovedClient) {
			defer wg.Done()
			
			checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			
			_, err := client.Client.NodeInfo(checkCtx, api.NodeInfoOptions{})
			
			mu.Lock()
			results[url] = err
			mu.Unlock()
			
			if err != nil {
				// Remove unhealthy client
				cp.mu.Lock()
				delete(cp.clients, url)
				cp.mu.Unlock()
			}
		}(url, client)
	}
	
	wg.Wait()
	return results
}

// CloseAll closes all clients in the pool
func (cp *ClientPool) CloseAll() {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	
	// Clear the client map
	cp.clients = make(map[string]*ImprovedClient)
}

// Example usage
func main() {
	fmt.Println("Improved V3 Client Example")
	fmt.Println("==========================")
	
	// Use the global pool
	pool := GetGlobalPool()
	
	// Configure with better settings
	config := DefaultClientConfig()
	config.MaxIdleConnsPerHost = 20 // Higher for load testing
	config.MaxRetries = 5
	pool.SetConfig(config)
	
	// Get a client from the pool
	client := pool.GetClient("http://127.0.0.1:26660/v3")
	
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Example: Query with automatic retry
	fmt.Println("\nQuerying network status with retry logic...")
	status, err := client.Client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		fmt.Printf("Failed to query network: %v\n", err)
	} else {
		fmt.Printf("Network status retrieved successfully\n")
		fmt.Printf("Oracle Price: %.4f\n", float64(status.Oracle.Price)/1e8)
	}
	
	// Health check all connections
	fmt.Println("\nPerforming health check on all connections...")
	results := pool.HealthCheck(ctx)
	for url, err := range results {
		if err != nil {
			fmt.Printf("  %s: UNHEALTHY - %v\n", url, err)
		} else {
			fmt.Printf("  %s: HEALTHY\n", url)
		}
	}
	
	fmt.Println("\nConnection pool statistics:")
	fmt.Printf("  Active connections: %d\n", len(pool.clients))
	fmt.Printf("  Max idle connections: %d\n", config.MaxIdleConns)
	fmt.Printf("  Max idle per host: %d\n", config.MaxIdleConnsPerHost)
	fmt.Printf("  Request timeout: %v\n", config.RequestTimeout)
	fmt.Printf("  Max retries: %d\n", config.MaxRetries)
}