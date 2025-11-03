package server

import (
	"fmt"
	"sync"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
)

// DatabaseManager manages a pool of open database connections
type DatabaseManager struct {
	mu    sync.RWMutex
	dbs   map[string]database.Beginner
	logger log.Logger
}

// NewDatabaseManager creates a new database connection manager
func NewDatabaseManager(logger log.Logger) *DatabaseManager {
	return &DatabaseManager{
		dbs:    make(map[string]database.Beginner),
		logger: logger,
	}
}

// GetDatabase gets or opens a database connection
func (dm *DatabaseManager) GetDatabase(dbPath string) (database.Beginner, error) {
	// Try read lock first for fast path
	dm.mu.RLock()
	if db, exists := dm.dbs[dbPath]; exists {
		dm.mu.RUnlock()
		return db, nil
	}
	dm.mu.RUnlock()

	// Need to open database - acquire write lock
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// Double-check in case another goroutine opened it
	if db, exists := dm.dbs[dbPath]; exists {
		return db, nil
	}

	// Open new database connection
	db, err := database.OpenBadger(dbPath, dm.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Cache the connection
	dm.dbs[dbPath] = db
	return db, nil
}

// Close closes all open database connections
func (dm *DatabaseManager) Close() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	var errs []error
	for path, db := range dm.dbs {
		if closer, ok := db.(interface{ Close() error }); ok {
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
