// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding_test

import (
	"io"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

func TestWriter_WriteBigInt(t *testing.T) {
	w := NewWriter(io.Discard)

	w.WriteBigInt(1, big.NewInt(-1))
	_, _, err := w.Reset(nil)
	require.Error(t, err)
}
