package chain_test

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	. "gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	testing2 "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

func TestLiteTokenTransactions(t *testing.T) {
	tokenUrl := types.String(protocol.AcmeUrl().String())
	db, err := database.Open("", true, nil)
	require.NoError(t, err)

	_, privKey, _ := ed25519.GenerateKey(nil)
	_, destPrivKey, _ := ed25519.GenerateKey(nil)

	batch := db.Begin(true)
	require.NoError(t, acctesting.CreateLiteTokenAccount(batch, tmed25519.PrivKey(privKey), acctesting.TestTokenAmount))
	require.NoError(t, batch.Commit())

	sponsorUrl := acctesting.AcmeLiteAddressStdPriv(privKey)
	liteAcct := new(protocol.LiteTokenAccount)
	require.NoError(t, db.Begin(true).Account(sponsorUrl).GetStateAs(liteAcct))

	//now move some tokens around
	destAddr := testing2.AcmeLiteAddressStdPriv(destPrivKey).String()
	gtx, err := testing2.BuildTestTokenTxGenTx(privKey, destAddr, 199)

	st, err := NewStateManager(db.Begin(true), protocol.BvnUrl(t.Name()), gtx)
	require.NoError(t, err)

	_, err = SendTokens{}.Validate(st, gtx)
	require.NoError(t, err)

	//pull the chains again
	tas := new(protocol.LiteTokenAccount)
	require.NoError(t, st.LoadUrlAs(st.OriginUrl, tas))
	require.Equal(t, *tokenUrl.AsString(), tas.TokenUrl.String(), "token url of state doesn't match expected")

}
