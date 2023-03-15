// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestRecordKeyBinary(t *testing.T) {
	k := RecordKey{int64(123), uint64(456), "Foo", [32]byte{7, 8, 9}, protocol.AccountUrl("foo"), protocol.AccountUrl("bar").WithTxID([32]byte{1})}
	b, err := k.MarshalBinary()
	require.NoError(t, err)

	var l RecordKey
	require.NoError(t, l.UnmarshalBinary(b))
	require.True(t, k.Equal(l))
}

func TestRecordKeyJSON(t *testing.T) {
	k := RecordKey{int64(123), uint64(456), "Foo", [32]byte{7, 8, 9}, protocol.AccountUrl("foo"), protocol.AccountUrl("bar").WithTxID([32]byte{1})}
	b, err := k.MarshalJSON()
	require.NoError(t, err)

	var l RecordKey
	require.NoError(t, l.UnmarshalJSON(b))
	require.True(t, k.Equal(l))
}
