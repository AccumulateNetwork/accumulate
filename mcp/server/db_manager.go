package server

import (
	"fmt"
	"sync"
	"time"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
)

// dbEntry holds a database connection with metadata
type dbEntry struct {
	db         database.Beginner
	lastAccess time.Time
}

// DatabaseManager manages a pool of open database connections
type DatabaseManager struct {
	mu         sync.RWMutex
	dbs        map[string]*dbEntry
	logger     log.Logger
	maxAge     time.Duration // Maximum age before eviction
	maxEntries int           // Maximum number of cached connections
}

// NewDatabaseManager creates a new database connection manager
func NewDatabaseManager(logger log.Logger) *DatabaseManager {
	dm := &DatabaseManager{
		dbs:        make(map[string]*dbEntry),
		logger:     logger,
		maxAge:     30 * time.Minute, // Evict connections unused for 30 minutes
		maxEntries: 10,               // Maximum 10 cached connections
	}

	// Start background eviction goroutine
	go dm.evictionLoop()

	return dm
}

// evictionLoop periodically removes stale connections
func (dm *DatabaseManager) evictionLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		dm.evictStale()
	}
}

// evictStale removes connections that haven't been used recently
func (dm *DatabaseManager) evictStale() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	now := time.Now()
	for path, entry := range dm.dbs {
		if now.Sub(entry.lastAccess) > dm.maxAge {
			if closer, ok := entry.db.(interface{ Close() error }); ok {
				_ = closer.Close()
			}
			delete(dm.dbs, path)
		}
	}
}

// GetDatabase gets or opens a database connection
func (dm *DatabaseManager) GetDatabase(dbPath string) (database.Beginner, error) {
	// Try read lock first for fast path
	dm.mu.RLock()
	if entry, exists := dm.dbs[dbPath]; exists {
		dm.mu.RUnlock()
		// Update access time under write lock
		dm.mu.Lock()
		entry.lastAccess = time.Now()
		dm.mu.Unlock()
		return entry.db, nil
	}
	dm.mu.RUnlock()

	// Need to open database - acquire write lock
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// Double-check in case another goroutine opened it
	if entry, exists := dm.dbs[dbPath]; exists {
		entry.lastAccess = time.Now()
		return entry.db, nil
	}

	// Check if we need to evict entries to stay under limit
	if len(dm.dbs) >= dm.maxEntries {
		dm.evictOldestLocked()
	}

	// Open new database connection
	db, err := database.OpenBadger(dbPath, dm.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Cache the connection
	dm.dbs[dbPath] = &dbEntry{
		db:         db,
		lastAccess: time.Now(),
	}
	return db, nil
}

// evictOldestLocked removes the oldest entry (must hold write lock)
func (dm *DatabaseManager) evictOldestLocked() {
	var oldestPath string
	var oldestTime time.Time

	for path, entry := range dm.dbs {
		if oldestPath == "" || entry.lastAccess.Before(oldestTime) {
			oldestPath = path
			oldestTime = entry.lastAccess
		}
	}

	if oldestPath != "" {
		if entry, exists := dm.dbs[oldestPath]; exists {
			if closer, ok := entry.db.(interface{ Close() error }); ok {
				_ = closer.Close()
			}
			delete(dm.dbs, oldestPath)
		}
	}
}

// Close closes all open database connections
func (dm *DatabaseManager) Close() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	var errs []error
	for path, entry := range dm.dbs {
		if closer, ok := entry.db.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, fmt.Errorf("failed to close %s: %w", path, err))
			}
		}
		delete(dm.dbs, path)
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing databases: %v", errs)
	}
	return nil
}

// Stats returns statistics about the database manager
func (dm *DatabaseManager) Stats() map[string]interface{} {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	entries := make([]map[string]interface{}, 0, len(dm.dbs))
	for path, entry := range dm.dbs {
		entries = append(entries, map[string]interface{}{
			"path":        path,
			"last_access": entry.lastAccess.Format(time.RFC3339),
			"age_seconds": time.Since(entry.lastAccess).Seconds(),
		})
	}

	return map[string]interface{}{
		"open_connections": len(dm.dbs),
		"max_entries":      dm.maxEntries,
		"max_age_minutes":  dm.maxAge.Minutes(),
		"entries":          entries,
	}
}
