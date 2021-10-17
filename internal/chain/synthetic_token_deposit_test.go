package chain_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"testing"

	. "github.com/AccumulateNetwork/accumulated/internal/chain"
	testing2 "github.com/AccumulateNetwork/accumulated/internal/testing"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/stretchr/testify/require"
)

func TestSynthTokenDeposit_Anon(t *testing.T) {
	appId := sha256.Sum256([]byte("anon"))
	tokenUrl := protocol.AcmeUrl().String()

	_, privKey, _ := ed25519.GenerateKey(nil)

	_, _, gtx, err := testing2.BuildTestSynthDepositGenTx(privKey)
	require.NoError(t, err)

	db := new(state.StateDB)
	require.NoError(t, db.Open("mem", appId[:], true, true))

	chainId := types.Bytes(gtx.ChainID).AsBytes32()
	se := &state.StateEntry{}
	se.ChainId = &chainId
	se.DB = db

	_, err = SyntheticTokenDeposit{}.DeliverTx(se, gtx)
	require.NoError(t, err)

	//try to extract the state to see if we have a valid account
	anonChain, err := db.GetCurrentEntry(chainId[:])
	require.NoError(t, err)

	tas := new(protocol.AnonTokenAccount)
	err = tas.UnmarshalBinary(anonChain.Entry)
	require.NoError(t, err)
	require.Equal(t, types.String(gtx.SigInfo.URL), tas.ChainUrl, "invalid chain header")
	require.Equalf(t, types.ChainTypeAnonTokenAccount, tas.Type, "chain state is not an anon account, it is %s", tas.ChainHeader.Type.Name())
	require.Equal(t, tokenUrl, tas.TokenUrl, "token url of state doesn't match expected")
	require.Equal(t, uint64(1), tas.TxCount)

	//now query the tx reference
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
