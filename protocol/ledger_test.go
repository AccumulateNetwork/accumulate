// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func TestPartitionSyntheticLedger_Add(t *testing.T) {
	ledger := new(PartitionSyntheticLedger)
	ledger.Received, ledger.Delivered = 3, 3

	foo := AccountUrl("foo")
	ledger.Add(false, 5, foo.WithTxID([32]byte{5}))
	ledger.Add(false, 6, foo.WithTxID([32]byte{6}))
	require.Equal(t, uint64(3), ledger.Delivered)
	require.Equal(t, uint64(6), ledger.Received)
	require.Len(t, ledger.Pending, 3)

	ledger.Add(false, 9, foo.WithTxID([32]byte{9}))
	ledger.Add(false, 8, foo.WithTxID([32]byte{8}))
	require.Equal(t, uint64(9), ledger.Received)
	require.Len(t, ledger.Pending, 6)

	require.Equal(t, []*url.TxID{
		nil,
		foo.WithTxID([32]byte{5}),
		foo.WithTxID([32]byte{6}),
		nil,
		foo.WithTxID([32]byte{8}),
		foo.WithTxID([32]byte{9}),
	}, ledger.Pending)

	ledger.Add(true, 4, foo.WithTxID([32]byte{4}))
	ledger.Add(true, 5, foo.WithTxID([32]byte{5}))
	require.Equal(t, uint64(5), ledger.Delivered)
	require.Equal(t, uint64(9), ledger.Received)
	require.Equal(t, []*url.TxID{
		foo.WithTxID([32]byte{6}),
		nil,
		foo.WithTxID([32]byte{8}),
		foo.WithTxID([32]byte{9}),
	}, ledger.Pending)
}
