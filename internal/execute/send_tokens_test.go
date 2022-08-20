package execute_test

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	. "gitlab.com/accumulatenetwork/accumulate/internal/execute"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/helpers"
)

func init() { acctesting.EnableDebugFeatures() }

func TestLiteTokenTransactions(t *testing.T) {
	db := database.OpenInMemory(nil)

	_, privKey, _ := ed25519.GenerateKey(nil)
	_, destPrivKey, _ := ed25519.GenerateKey(nil)

	batch := db.Begin(true)
	require.NoError(t, acctesting.CreateLiteTokenAccount(batch, tmed25519.PrivKey(privKey), protocol.AcmeFaucetAmount))
	require.NoError(t, batch.Commit())

	sponsorUrl := acctesting.AcmeLiteAddressStdPriv(privKey)
	var liteAcct *protocol.LiteTokenAccount
	require.NoError(t, db.Begin(true).Account(sponsorUrl).GetStateAs(&liteAcct))

	//now move some tokens around
	destAddr := acctesting.AcmeLiteAddressStdPriv(destPrivKey).String()
	gtx, err := acctesting.BuildTestTokenTxGenTx(privKey, destAddr, 199)
	require.NoError(t, err)

	st, d := LoadStateManagerForTest(t, db, gtx)
	defer st.Discard()

	_, err = SendTokens{}.Validate(st, d)
	require.NoError(t, err)

	//pull the chains again
	var tas *protocol.LiteTokenAccount
	require.NoError(t, st.LoadUrlAs(st.OriginUrl, &tas))
	require.Equal(t, protocol.AcmeUrl().String(), tas.TokenUrl.String(), "token url of state doesn't match expected")

}
