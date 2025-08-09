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

// ClientPoolEntry tracks a client and its last use time for cleanup
type ClientPoolEntry struct {
	Client     *jsonrpc.Client
	LastUsed   time.Time
	CreateTime time.Time
}

// Client pool for v3 connections - prevents connection exhaustion
var (
	clientPool    = make(map[string]*ClientPoolEntry)
	clientMu      sync.RWMutex
	maxPoolSize   = 100 // Maximum number of clients in pool
	clientTTL     = 5 * time.Minute // Time to live for unused clients
	cleanupTicker *time.Ticker
	cleanupOnce   sync.Once
)

// startCleanupRoutine starts a background routine to clean up old clients
func startCleanupRoutine() {
	cleanupOnce.Do(func() {
		cleanupTicker = time.NewTicker(1 * time.Minute)
		go func() {
			for range cleanupTicker.C {
				cleanupOldClients()
			}
		}()
	})
}

// cleanupOldClients removes clients that haven't been used recently
func cleanupOldClients() {
	clientMu.Lock()
	defer clientMu.Unlock()
	
	now := time.Now()
	for url, entry := range clientPool {
		if now.Sub(entry.LastUsed) > clientTTL {
			// Close idle connections
			if transport, ok := entry.Client.Client.Transport.(*http.Transport); ok {
				transport.CloseIdleConnections()
			}
			delete(clientPool, url)
		}
	}
	
	// If pool is too large, remove oldest entries
	if len(clientPool) > maxPoolSize {
		// Find and remove oldest entries
		type urlAge struct {
			url string
			age time.Time
		}
		var entries []urlAge
		for url, entry := range clientPool {
			entries = append(entries, urlAge{url, entry.LastUsed})
		}
		
		// Sort by age and remove oldest
		for i := 0; i < len(entries)-maxPoolSize; i++ {
			if transport, ok := clientPool[entries[i].url].Client.Client.Transport.(*http.Transport); ok {
				transport.CloseIdleConnections()
			}
			delete(clientPool, entries[i].url)
		}
	}
}

// GetPooledClient returns an optimized, reusable client for the given server URL
// This prevents connection exhaustion and improves performance
func GetPooledClient(serverURL string) *jsonrpc.Client {
	// Start cleanup routine on first use
	startCleanupRoutine()
	
	clientMu.RLock()
	if entry, exists := clientPool[serverURL]; exists {
		entry.LastUsed = time.Now()
		clientMu.RUnlock()
		return entry.Client
	}
	clientMu.RUnlock()
	
	clientMu.Lock()
	defer clientMu.Unlock()
	
	// Double-check after acquiring write lock
	if entry, exists := clientPool[serverURL]; exists {
		entry.LastUsed = time.Now()
		return entry.Client
	}
	
	// Check pool size limit before creating new client
	if len(clientPool) >= maxPoolSize {
		// Find and remove oldest entry
		var oldestURL string
		var oldestTime time.Time
		for url, entry := range clientPool {
			if oldestTime.IsZero() || entry.LastUsed.Before(oldestTime) {
				oldestURL = url
				oldestTime = entry.LastUsed
			}
		}
		
		if oldestURL != "" {
			if transport, ok := clientPool[oldestURL].Client.Client.Transport.(*http.Transport); ok {
				transport.CloseIdleConnections()
			}
			delete(clientPool, oldestURL)
		}
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
	
	entry := &ClientPoolEntry{
		Client:     client,
		LastUsed:   time.Now(),
		CreateTime: time.Now(),
	}
	
	clientPool[serverURL] = entry
	return client
}

// QueryWithRetry performs a query with automatic retry logic for transient failures
func QueryWithRetry(ctx context.Context, client *jsonrpc.Client, fn func() error) error {
	var lastErr error
	maxRetries := 3
	baseDelay := 1 * time.Second
	
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter
			delay := time.Duration(attempt) * baseDelay
			jitter := time.Duration(float64(delay) * 0.1) // 10% jitter
			delay = delay + jitter
			
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
	
	// Check for context errors (not retryable)
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}
	
	// Network timeout errors
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}
	
	// Connection errors that are typically transient
	errStr := strings.ToLower(err.Error())
	retryableStrings := []string{
		"connection refused",
		"connection reset",
		"eof",
		"broken pipe",
		"no such host",
		"network is unreachable",
		"timeout",
		"temporary failure",
		"too many open files", // Connection exhaustion
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

// CleanupClientPool properly closes all pooled clients and their connections
func CleanupClientPool() {
	clientMu.Lock()
	defer clientMu.Unlock()
	
	// Stop cleanup routine
	if cleanupTicker != nil {
		cleanupTicker.Stop()
	}
	
	// Properly close all connections
	for url, entry := range clientPool {
		if transport, ok := entry.Client.Client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
		delete(clientPool, url)
	}
	
	// Reset the pool
	clientPool = make(map[string]*ClientPoolEntry)
}

// GetDefaultClient returns an optimized client for the default test server
func GetDefaultClient() *jsonrpc.Client {
	return GetPooledClient("http://127.0.0.1:26660/v3")
}

// GetPoolStats returns statistics about the client pool
func GetPoolStats() (size int, oldest time.Time, newest time.Time) {
	clientMu.RLock()
	defer clientMu.RUnlock()
	
	size = len(clientPool)
	
	for _, entry := range clientPool {
		if oldest.IsZero() || entry.CreateTime.Before(oldest) {
			oldest = entry.CreateTime
		}
		if newest.IsZero() || entry.CreateTime.After(newest) {
			newest = entry.CreateTime
		}
	}
	
	return size, oldest, newest
}