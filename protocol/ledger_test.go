package protocol

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubnetSyntheticLedger_Add(t *testing.T) {
	ledger := new(SubnetSyntheticLedger)
	ledger.Received, ledger.Delivered = 3, 3

	ledger.Add(false, 5, [32]byte{5})
	ledger.Add(false, 6, [32]byte{6})
	require.Equal(t, uint64(3), ledger.Delivered)
	require.Equal(t, uint64(6), ledger.Received)
	require.Len(t, ledger.Pending, 3)

	ledger.Add(false, 9, [32]byte{9})
	ledger.Add(false, 8, [32]byte{8})
	require.Equal(t, uint64(9), ledger.Received)
	require.Len(t, ledger.Pending, 6)

	require.Equal(t, [][32]byte{
		{0},
		{5},
		{6},
		{0},
		{8},
		{9},
	}, ledger.Pending)

	ledger.Add(true, 4, [32]byte{4})
	ledger.Add(true, 5, [32]byte{5})
	require.Equal(t, uint64(5), ledger.Delivered)
	require.Equal(t, uint64(9), ledger.Received)
	require.Equal(t, [][32]byte{
		{6},
		{0},
		{8},
		{9},
	}, ledger.Pending)
}
