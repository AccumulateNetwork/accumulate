// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package record_test

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
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

	k = NewKey(1)
	b, err = k.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, "010102", hex.EncodeToString(b))
}

func TestKeyJSON(t *testing.T) {
	k := NewKey(int64(123), uint64(456), "Foo", [32]byte{7, 8, 9}, protocol.AccountUrl("foo"), protocol.AccountUrl("bar").WithTxID([32]byte{1}))
	b, err := k.MarshalJSON()
	require.NoError(t, err)

	var l Key
	require.NoError(t, l.UnmarshalJSON(b))
	require.True(t, k.Equal(&l))

	k = NewKey(1)
	b, err = k.MarshalJSON()
	require.NoError(t, err)
	require.Equal(t, `[{"int":1}]`, string(b))
}

func BenchmarkKey_Append(b *testing.B) {
	key := NewKey(1)
	var x *record.Key
	for range b.N {
		x = key.Append("foo")
	}
	require.NotNil(b, x)
}

func BenchmarkKey_Compare(b *testing.B) {
	i, j := NewKey(1), NewKey(2)
	var c int
	for range b.N {
		c = i.Compare(j)
	}
	require.Equal(b, -1, c)
}

func BenchmarkKey_Hash(b *testing.B) {
	key := NewKey("foo", 1)
	var x [32]byte
	for range b.N {
		key := *key // Prevent Hash from saving the result
		x = key.Hash()
	}
	require.NotZero(b, x)
}

func BenchmarkKey_Copy(b *testing.B) {
	key := NewKey("foo", 1)
	var x *record.Key
	for range b.N {
		x = key.Copy()
	}
	require.NotNil(b, x)
}

func BenchmarkKey_Equal(b *testing.B) {
	i, j := NewKey(1), NewKey(2)
	var c bool
	for range b.N {
		c = i.Equal(j)
	}
	require.False(b, c)
}

func BenchmarkKey_MarshalJSON(b *testing.B) {
	key := NewKey("foo", 1)
	var err error
	for range b.N {
		_, err = key.MarshalJSON()
	}
	require.NoError(b, err)
}

func BenchmarkKey_MarshalBinary(b *testing.B) {
	// BenchmarkKey_MarshalBinary       6599190               178.6 ns/op             0 B/op          0 allocs/op
	// BenchmarkKey_MarshalBinary       2352801               518.5 ns/op           360 B/op          9 allocs/op
	key := NewKey("foo", 1)
	var err error
	for range b.N {
		_, err = key.MarshalBinary()
	}
	require.NoError(b, err)
}
