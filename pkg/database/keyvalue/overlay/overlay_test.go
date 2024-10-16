// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package overlay

import (
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/kvtest"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
)

func open(*testing.T) kvtest.Opener {
	db := Open(memory.New(nil), memory.New(nil))
	return func() (keyvalue.Beginner, error) {
		return db, nil
	}
}

func TestDatabase(t *testing.T) {
	kvtest.TestDatabase(t, open(t))
}

func TestIsolation(t *testing.T) {
	t.Skip("Not supported by the underlying databases")
	kvtest.TestIsolation(t, open(t))
}

func TestSubBatch(t *testing.T) {
	kvtest.TestSubBatch(t, open(t))
}

func TestPrefix(t *testing.T) {
	kvtest.TestPrefix(t, open(t))
}

func TestDelete(t *testing.T) {
	kvtest.TestDelete(t, open(t))
}
