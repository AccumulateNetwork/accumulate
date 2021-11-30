package chain_test

import (
	"crypto/ed25519"
	"fmt"
	"testing"

	. "github.com/AccumulateNetwork/accumulate/internal/chain"
	testing2 "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/stretchr/testify/require"
)

func TestSynthTokenDeposit_Anon(t *testing.T) {
	tokenUrl := protocol.AcmeUrl().String()

	_, privKey, _ := ed25519.GenerateKey(nil)

	_, _, gtx, err := testing2.BuildTestSynthDepositGenTx(privKey)
	require.NoError(t, err)

	db := new(state.StateDB)
	require.NoError(t, db.Open("mem", true, true, nil))

	st, err := NewStateManager(db.Begin(), gtx)
	require.ErrorIs(t, err, storage.ErrNotFound)

	err = SyntheticTokenDeposit{}.Validate(st, gtx)
	require.NoError(t, err)

	//try to extract the state to see if we have a valid account
	tas := new(protocol.AnonTokenAccount)
	require.NoError(t, st.LoadAs(st.SponsorChainId, tas))
	require.Equal(t, types.String(gtx.SigInfo.URL), tas.ChainUrl, "invalid chain header")
	require.Equalf(t, types.ChainTypeLiteTokenAccount, tas.Type, "chain state is not an anon account, it is %s", tas.ChainHeader.Type.Name())
	require.Equal(t, tokenUrl, tas.TokenUrl, "token url of state doesn't match expected")
	require.Equal(t, uint64(1), tas.TxCount)

	//now query the tx reference
	refUrl := st.SponsorUrl.JoinPath(fmt.Sprint(tas.TxCount - 1))
	txRef := new(state.TxReference)
	require.NoError(t, st.LoadUrlAs(refUrl, txRef))
	require.Equal(t, types.String(refUrl.String()), txRef.ChainUrl, "chain header expected transaction reference")
	require.Equal(t, gtx.TransactionHash(), txRef.TxId[:], "txid doesn't match")
}
