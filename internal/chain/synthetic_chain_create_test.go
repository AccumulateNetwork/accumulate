package chain_test

import (
	"testing"

	. "github.com/AccumulateNetwork/accumulate/internal/chain"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/stretchr/testify/require"
)

func TestSyntheticChainCreate_MultiSlash(t *testing.T) {
	db := new(state.StateDB)
	require.NoError(t, db.Open("mem", true, true))

	fooKey := generateKey()
	require.NoError(t, acctesting.CreateADI(db, fooKey, "foo"))
	_, _, err := db.WriteStates(0)
	require.NoError(t, err)

	book, err := url.Parse("foo/ssg0")
	require.NoError(t, err)

	account := state.NewTokenAccount("foo/bar/baz", "ACME")
	account.SigSpecId = types.Bytes(book.ResourceChain()).AsBytes32()
	body := new(protocol.SyntheticCreateChain)
	body.Cause[0] = 1
	require.NoError(t, body.Create(account))

	tx, err := transactions.New("foo", edSigner(fooKey, 1), body)
	require.NoError(t, err)

	st, err := NewStateManager(db, tx)
	require.NoError(t, err)

	err = SyntheticCreateChain{}.DeliverTx(st, tx)
	require.EqualError(t, err, `ChainTypeTokenAccount cannot contain more than one slash in its URL`)
}
