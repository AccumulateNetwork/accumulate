// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() { acctesting.EnableDebugFeatures() }

func TestSynthTokenDeposit_Lite(t *testing.T) {
	t.Skip("TODO Broken")

	tokenUrl := protocol.AcmeUrl().String()

	var gtx *Delivery
	// _, _, gtx, err := acctesting.BuildTestSynthDepositGenTx()
	// require.NoError(t, err)

	db := database.OpenInMemory(nil)

	batch := db.Begin(true)
	defer batch.Discard()
	require.NoError(t, acctesting.CreateTokenAccount(batch, protocol.FaucetUrl.String(), protocol.ACME, 1e9, true))

	st, d := LoadStateManagerForTest(t, db, nil)
	defer st.Discard()

	_, err := SyntheticDepositTokens{}.Validate(st, d)
	require.NoError(t, err)

	//try to extract the state to see if we have a valid account
	var tas *protocol.LiteTokenAccount
	require.NoError(t, st.LoadUrlAs(st.OriginUrl, &tas))
	require.Equal(t, gtx.Transaction.Header.Principal.String(), tas.Url.String(), "invalid chain header")
	require.Equal(t, tokenUrl, tas.TokenUrl.String(), "token url of state doesn't match expected")

}
