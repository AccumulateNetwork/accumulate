package chain_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	testing2 "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

func TestSynthTokenDeposit_Lite(t *testing.T) {
	tokenUrl := protocol.AcmeUrl().String()

	_, _, gtx, err := testing2.BuildTestSynthDepositGenTx()
	require.NoError(t, err)

	db, err := database.Open("", true, nil)
	require.NoError(t, err)

	st, err := NewStateManager(db.Begin(true), protocol.BvnUrl(t.Name()), gtx)
	require.ErrorIs(t, err, storage.ErrNotFound)

	_, err = SyntheticDepositTokens{}.Validate(st, gtx)
	require.NoError(t, err)

	//try to extract the state to see if we have a valid account
	tas := new(protocol.LiteTokenAccount)
	require.NoError(t, st.LoadUrlAs(st.OriginUrl, tas))
	require.Equal(t, gtx.Transaction.Origin.String(), tas.Url.String(), "invalid chain header")
	require.Equal(t, tokenUrl, tas.TokenUrl.String(), "token url of state doesn't match expected")

}
