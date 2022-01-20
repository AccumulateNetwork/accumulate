package chain_test

import (
	"testing"

	. "github.com/AccumulateNetwork/accumulate/internal/chain"
	"github.com/AccumulateNetwork/accumulate/internal/database"
	testing2 "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/stretchr/testify/require"
)

func TestSynthTokenDeposit_Lite(t *testing.T) {
	tokenUrl := protocol.AcmeUrl().String()

	_, _, gtx, err := testing2.BuildTestSynthDepositGenTx()
	require.NoError(t, err)

	db, err := database.Open("", true, nil)
	require.NoError(t, err)

	st, err := NewStateManager(db.Begin(), protocol.BvnUrl(t.Name()), gtx)
	require.ErrorIs(t, err, storage.ErrNotFound)

	_, err = SyntheticDepositTokens{}.Validate(st, gtx)
	require.NoError(t, err)

	//try to extract the state to see if we have a valid account
	tas := new(protocol.LiteTokenAccount)
	require.NoError(t, st.LoadUrlAs(st.OriginUrl, tas))
	require.Equal(t, types.String(gtx.Transaction.Origin.String()), tas.ChainUrl, "invalid chain header")
	require.Equalf(t, types.AccountTypeLiteTokenAccount, tas.Type, "chain state is not a lite account, it is %s", tas.ChainHeader.Type.Name())
	require.Equal(t, tokenUrl, tas.TokenUrl, "token url of state doesn't match expected")

}
