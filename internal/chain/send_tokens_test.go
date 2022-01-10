package chain_test

import (
	"crypto/ed25519"
	"testing"

	. "github.com/AccumulateNetwork/accumulate/internal/chain"
	"github.com/AccumulateNetwork/accumulate/internal/database"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	testing2 "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
)

func TestLiteTokenTransactions(t *testing.T) {
	tokenUrl := types.String(protocol.AcmeUrl().String())
	db, err := database.Open("", true, nil)
	require.NoError(t, err)

	_, privKey, _ := ed25519.GenerateKey(nil)
	_, destPrivKey, _ := ed25519.GenerateKey(nil)

	batch := db.Begin()
	require.NoError(t, acctesting.CreateLiteTokenAccount(batch, tmed25519.PrivKey(privKey), 5e4))
	require.NoError(t, batch.Commit())

	sponsorUrl := acctesting.AcmeLiteAddressStdPriv(privKey)
	liteAcct := new(protocol.LiteTokenAccount)
	require.NoError(t, db.Begin().Record(sponsorUrl).GetStateAs(liteAcct))

	//now move some tokens around
	destAddr := testing2.AcmeLiteAddressStdPriv(destPrivKey).String()
	gtx, err := testing2.BuildTestTokenTxGenTx(privKey, destAddr, 199)

	st, err := NewStateManager(db.Begin(), protocol.BvnUrl(t.Name()), gtx)
	require.NoError(t, err)

	err = SendTokens{}.Validate(st, gtx)
	require.NoError(t, err)

	//pull the chains again
	tas := new(protocol.LiteTokenAccount)
	require.NoError(t, st.LoadUrlAs(st.OriginUrl, tas))
	require.Equal(t, tokenUrl, types.String(tas.TokenUrl), "token url of state doesn't match expected")

}
