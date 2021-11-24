package chain_test

import (
	"crypto/ed25519"
	"fmt"
	"testing"

	. "github.com/AccumulateNetwork/accumulate/internal/chain"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	testing2 "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	anon "github.com/AccumulateNetwork/accumulate/types/anonaddress"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
)

func TestAnonTokenTransactions(t *testing.T) {
	tokenUrl := types.String(protocol.AcmeUrl().String())
	db := &state.StateDB{}
	err := db.Open("mem", true, true)
	require.NoError(t, err)

	_, privKey, _ := ed25519.GenerateKey(nil)
	_, destPrivKey, _ := ed25519.GenerateKey(nil)

	dbTx := db.Begin()
	require.NoError(t, acctesting.CreateAnonTokenAccount(dbTx, tmed25519.PrivKey(privKey), 5e4))
	_, _, err = dbTx.Commit(0)
	require.NoError(t, err)

	sponsorAddr := anon.GenerateAcmeAddress(privKey[32:])
	anonChain, err := db.GetPersistentEntry(types.GetChainIdFromChainPath(&sponsorAddr).Bytes(), false)
	require.NoError(t, err)
	anonAcct := new(protocol.AnonTokenAccount)
	require.NoError(t, anonChain.As(anonAcct))

	//now move some tokens around
	destAddr := anon.GenerateAcmeAddress(destPrivKey[32:])
	gtx, err := testing2.BuildTestTokenTxGenTx(privKey, destAddr, 199)

	st, err := NewStateManager(db.Begin(), gtx)
	require.NoError(t, err)

	err = WithdrawTokens{}.Validate(st, gtx)
	require.NoError(t, err)

	//pull the chains again
	tas := new(protocol.AnonTokenAccount)
	require.NoError(t, st.LoadAs(st.SponsorChainId, tas))
	require.Equal(t, tokenUrl, types.String(tas.TokenUrl), "token url of state doesn't match expected")
	require.Equal(t, uint64(2), tas.TxCount, "expected a token transaction count of 2")

	refUrl := st.SponsorUrl.JoinPath(fmt.Sprint(tas.TxCount - 1))
	txRef := new(state.TxReference)
	require.NoError(t, st.LoadUrlAs(refUrl, txRef))
	require.Equal(t, types.String(refUrl.String()), txRef.ChainUrl, "chain header expected transaction reference")
	require.Equal(t, gtx.TransactionHash(), txRef.TxId[:], "txid doesn't match")
}
