// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package record_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestKeyBinary(t *testing.T) {
	k := NewKey(int64(123), uint64(456), "Foo", [32]byte{7, 8, 9}, protocol.AccountUrl("foo"), protocol.AccountUrl("bar").WithTxID([32]byte{1}))
	b, err := k.MarshalBinary()
	require.NoError(t, err)

	var l Key
	require.NoError(t, l.UnmarshalBinary(b))
	require.True(t, k.Equal(&l))
}

func TestKeyJSON(t *testing.T) {
	k := NewKey(int64(123), uint64(456), "Foo", [32]byte{7, 8, 9}, protocol.AccountUrl("foo"), protocol.AccountUrl("bar").WithTxID([32]byte{1}))
	b, err := k.MarshalJSON()
	require.NoError(t, err)

	var l Key
	require.NoError(t, l.UnmarshalJSON(b))
	require.True(t, k.Equal(&l))
}
