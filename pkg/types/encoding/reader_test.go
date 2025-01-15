// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestReadLargeValue(t *testing.T) {
	acctesting.SkipCI(t, "Times out")

	// Create an account snapshot with millions of chain entries
	account := new(snapshot.Account)
	account.Url = protocol.AccountUrl("foo")
	account.Main = &protocol.LiteTokenAccount{Url: account.Url}
	account.Chains = []*snapshot.Chain{
		{Name: "main", Type: merkle.ChainTypeTransaction},
		{Name: "scratch", Type: merkle.ChainTypeTransaction},
		{Name: "signature", Type: merkle.ChainTypeTransaction},
	}
	for _, c := range account.Chains {
		for i := 0; i < 50_000; i++ {
			s := new(merkle.State)
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
