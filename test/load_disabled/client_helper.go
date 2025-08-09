package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

// Client pool for v3 connections - prevents connection exhaustion
var (
	clientPool = make(map[string]*jsonrpc.Client)
	clientMu   sync.RWMutex
)

// GetPooledClient returns an optimized, reusable client for the given server URL
// This prevents connection exhaustion and improves performance
func GetPooledClient(serverURL string) *jsonrpc.Client {
	clientMu.RLock()
	if client, exists := clientPool[serverURL]; exists {
		clientMu.RUnlock()
		return client
	}
	clientMu.RUnlock()
	
	clientMu.Lock()
	defer clientMu.Unlock()
	
	// Double-check after acquiring write lock
	if client, exists := clientPool[serverURL]; exists {
		return client
	}
	
	// Create client with optimized transport
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20, // Higher for load testing
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableKeepAlives:   false, // Keep connections alive for reuse
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	
	client := jsonrpc.NewClient(serverURL)
	client.Client.Transport = transport
	client.Client.Timeout = 30 * time.Second
	
	clientPool[serverURL] = client
	return client
}

// QueryWithRetry performs a query with automatic retry logic for transient failures
func QueryWithRetry(ctx context.Context, client *jsonrpc.Client, fn func() error) error {
	var lastErr error
	maxRetries := 3
	
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := time.Duration(attempt) * time.Second
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		
		err := fn()
		if err == nil {
			return nil
		}
		
		// Check if error is retryable
		if !IsRetryableError(err) {
			return err
		}
		
		lastErr = err
	}
	
	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// IsRetryableError determines if an error should trigger a retry
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}
	
	// Network timeout errors
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}
	
	// Connection errors that are typically transient
	errStr := err.Error()
	retryableStrings := []string{
		"connection refused",
		"connection reset",
		"EOF",
		"broken pipe",
		"no such host",
		"network is unreachable",
		"timeout",
	}
	
	for _, s := range retryableStrings {
		if strings.Contains(errStr, s) {
			return true
		}
	}
	
	return false
}

// CreateContextWithTimeout creates a context with a reasonable timeout
func CreateContextWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout == 0 {
		timeout = 30 * time.Second // Default timeout
	}
	return context.WithTimeout(context.Background(), timeout)
}

// SafeQuery wraps a query with timeout and retry logic
func SafeQuery(client *jsonrpc.Client, fn func(context.Context) error) error {
	ctx, cancel := CreateContextWithTimeout(30 * time.Second)
	defer cancel()
	
	return QueryWithRetry(ctx, client, func() error {
		return fn(ctx)
	})
}

// CleanupClientPool closes all pooled clients (call at end of test)
func CleanupClientPool() {
	clientMu.Lock()
	defer clientMu.Unlock()
	
	// Clear the pool
	clientPool = make(map[string]*jsonrpc.Client)
}

// GetDefaultClient returns an optimized client for the default test server
func GetDefaultClient() *jsonrpc.Client {
	return GetPooledClient("http://127.0.0.1:26660/v3")
}

// Example usage patterns for test files:
//
// 1. Simple usage with default server:
//    client := GetDefaultClient()
//
// 2. Custom server with pooling:
//    client := GetPooledClient("http://custom-server:26660/v3")
//
// 3. Query with retry:
//    err := SafeQuery(client, func(ctx context.Context) error {
//        _, err := client.NodeInfo(ctx, api.NodeInfoOptions{})
//        return err
//    })
//
// 4. Manual retry with custom logic:
//    err := QueryWithRetry(ctx, client, func() error {
//        return myCustomOperation()
//    })
//
// 5. Cleanup after tests:
//    defer CleanupClientPool()