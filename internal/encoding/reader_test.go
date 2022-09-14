package encoding_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
)

func TestReadLargeValue(t *testing.T) {
	// Create an account snapshot with millions of chain entries
	account := new(snapshot.Account)
	account.Url = protocol.AccountUrl("foo")
	account.Main = &protocol.LiteTokenAccount{Url: account.Url}
	account.Chains = []*managed.Snapshot{
		{Name: "main", Type: managed.ChainTypeTransaction},
		{Name: "scratch", Type: managed.ChainTypeTransaction},
		{Name: "signature", Type: managed.ChainTypeTransaction},
	}
	for _, c := range account.Chains {
		for i := 0; i < 50_000; i++ {
			s := new(managed.MerkleState)
			for i := 0; i < 256; i++ {
				s.HashList = append(s.HashList, make([]byte, 32))
			}
			c.MarkPoints = append(c.MarkPoints, s)
		}
	}

	// Marshal and verify that it exceeds the max value size
	b, err := account.MarshalBinary()
	require.NoError(t, err)
	require.Greater(t, len(b), encoding.MaxValueSize)

	// Verify that it can be unmarshalled
	require.NoError(t, new(snapshot.Account).UnmarshalBinary(b))
}
