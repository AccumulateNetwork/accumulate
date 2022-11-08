// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pmt_test

import (
	"crypto/sha256"
	"testing"

	. "gitlab.com/accumulatenetwork/accumulate/internal/database/smt/pmt"
)

func TestValue_Equal(t *testing.T) {
	value := new(Value)
	value.Key = sha256.Sum256([]byte{1})
	value.Hash = sha256.Sum256([]byte{2})

	value2 := new(Value)
	value2.Key = sha256.Sum256([]byte{1})
	value2.Hash = sha256.Sum256([]byte{2})

	if !value.Equal(value2) {
		t.Error("value should be the same")
	}
}
