package chain_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	testing2 "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestSynthTokenDeposit_Lite(t *testing.T) {
	t.Skip("TODO Broken")

	tokenUrl := protocol.AcmeUrl().String()

	var gtx *Delivery
	// _, _, gtx, err := testing2.BuildTestSynthDepositGenTx()
	// require.NoError(t, err)

	db := database.OpenInMemory(nil)

	batch := db.Begin(true)
	defer batch.Discard()
	require.NoError(t, testing2.CreateTokenAccount(batch, protocol.FaucetUrl.String(), protocol.ACME, 1e9, true))

	st, d := NewStateManagerForTest(t, db, nil)
	defer st.Discard()

	_, err := SyntheticDepositTokens{}.Validate(st, d)
	require.NoError(t, err)

	//try to extract the state to see if we have a valid account
	var tas *protocol.LiteTokenAccount
	require.NoError(t, st.LoadUrlAs(st.OriginUrl, &tas))
	require.Equal(t, gtx.Transaction.Header.Principal.String(), tas.Url.String(), "invalid chain header")
	require.Equal(t, tokenUrl, tas.TokenUrl.String(), "token url of state doesn't match expected")

}
