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

func open(testing.TB) kvtest.Opener {
	db := Open(memory.New(nil), memory.New(nil))
	return func() (keyvalue.Beginner, error) {
		return db, nil
	}
}

func TestSuite(t *testing.T) {
	kvtest.TestSuite(t, open(t))
}
