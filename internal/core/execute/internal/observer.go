// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package internal

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
)

// databaseObserver is a minimal observer for database changes.
// It implements the database.Observer interface.
type databaseObserver struct{}

var _ database.Observer = databaseObserver{}

// NewDatabaseObserver creates a new database observer.
func NewDatabaseObserver() database.Observer {
	return databaseObserver{}
}

// DidChangeAccount is called when an account is changed.
// This implementation is minimal and does not perform any special actions.
func (databaseObserver) DidChangeAccount(batch *database.Batch, account *database.Account) (hash.Hasher, error) {
	// Return nil hasher - no special handling needed for DAG-BFT
	return nil, nil
}
