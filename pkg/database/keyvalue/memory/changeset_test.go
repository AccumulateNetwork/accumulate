// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package memory

import (
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/kvtest"
)

func open() kvtest.Opener {
	// Reuse the same in-memory database each time
	db := New(nil)
	return func() (keyvalue.Beginner, error) { return db, nil }
}

func TestDatabase(t *testing.T) {
	kvtest.TestDatabase(t, open())
}

func TestIsolation(t *testing.T) {
	t.Skip("Isolation not supported")
	kvtest.TestIsolation(t, open())
}

func TestSubBatch(t *testing.T) {
	kvtest.TestSubBatch(t, open())
}

func TestPrefix(t *testing.T) {
	kvtest.TestPrefix(t, open())
}

func TestDelete(t *testing.T) {
	kvtest.TestDelete(t, open())
}
