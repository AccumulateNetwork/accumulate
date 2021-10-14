package chain_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"testing"

	. "github.com/AccumulateNetwork/accumulated/internal/chain"
	acctesting "github.com/AccumulateNetwork/accumulated/internal/testing"
	testing2 "github.com/AccumulateNetwork/accumulated/internal/testing"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	anon "github.com/AccumulateNetwork/accumulated/types/anonaddress"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/stretchr/testify/require"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
)

func TestAnonTokenTransactions(t *testing.T) {
	appId := sha256.Sum256([]byte("anon"))
	tokenUrl := types.String(protocol.AcmeUrl().String())
	db := &state.StateDB{}
	err := db.Open("mem", appId[:], true, true)
	require.NoError(t, err)

	_, privKey, _ := ed25519.GenerateKey(nil)
	_, destPrivKey, _ := ed25519.GenerateKey(nil)

	require.NoError(t, acctesting.CreateAnonTokenAccount(db, tmed25519.PrivKey(privKey), 5e4))
	sponsorAddr := anon.GenerateAcmeAddress(privKey[32:])
	anonChain, err := db.GetCurrentEntry(types.GetChainIdFromChainPath(&sponsorAddr).Bytes())
	require.NoError(t, err)
	anonAcct := new(state.TokenAccount)
	require.NoError(t, anonChain.As(anonAcct))

	se := &state.StateEntry{}
	se.DB = db

	//now move some tokens around
	destAddr := anon.GenerateAcmeAddress(destPrivKey[32:])
	gtx, err := testing2.BuildTestTokenTxGenTx(&destPrivKey, destAddr, 199)

	chainId := types.Bytes(gtx.ChainID).AsBytes32()
	se.ChainId = &chainId
	se.AdiChain = &chainId
	se.ChainState = anonChain
	se.AdiState = anonChain
	se.AdiHeader = &anonAcct.ChainHeader
	se.ChainHeader = &anonAcct.ChainHeader
	_, err = TokenTx{}.DeliverTx(se, gtx)
	require.NoError(t, err)

	//pull the chains again
	anonChain, err = db.GetCurrentEntry(chainId[:])
	require.NoError(t, err)

	tas := state.TokenAccount{}
	err = tas.UnmarshalBinary(anonChain.Entry)
	require.NoError(t, err)
	require.Equal(t, tokenUrl, tas.TokenUrl.String, "token url of state doesn't match expected")
	require.Equal(t, uint64(2), tas.TxCount, "expected a token transaction count of 2")

	refUrl := fmt.Sprintf("%s/%d", gtx.SigInfo.URL, tas.TxCount-1)
	refChainId := types.GetChainIdFromChainPath(&refUrl)
	refChainObject, err := db.GetCurrentEntry(refChainId.Bytes())
	require.NoError(t, err)

	txRef := state.TxReference{}
	err = txRef.UnmarshalBinary(refChainObject.Entry)
	require.NoError(t, err)
	require.Equal(t, types.String(refUrl), txRef.ChainUrl, "chain header expected transaction reference")
	require.Equal(t, gtx.TransactionHash(), txRef.TxId[:], "txid doesn't match")
}
