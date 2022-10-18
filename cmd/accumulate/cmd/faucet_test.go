// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	testMatrix.addTest(testCase5_1)
}

func testCase5_1(t *testing.T, tc *testCmd) {

	beenFauceted := make([]bool, len(liteAccounts))
	//test to see if things have already been fauceted...
	for i := range liteAccounts {
		bal, err := testGetBalance(t, tc, liteAccounts[i])
		if err == nil {
			balance, err := strconv.Atoi(bal)
			require.NoError(t, err)
			beenFauceted[i] = balance > 0
			continue
		}

		jerr := new(JsonRpcError)
		require.ErrorAs(t, err, &jerr)
		require.Equalf(t, client.ErrCodeNotFound, int(jerr.Err.Code), "Expected not found, got %q, %v", jerr.Err.Message, jerr.Err.Data)
	}

	var results []string
	for i := range liteAccounts {
		commandLine := fmt.Sprintf("faucet %s", liteAccounts[i])
		r, err := tc.execute(t, commandLine)
		require.NoError(t, err)
		results = append(results, r)
	}

	//wait for settlement
	for _, r := range results {
		var res ActionResponse
		require.NoError(t, json.Unmarshal([]byte(r), &res))
		_, err := waitForTxnUsingHash(res.TransactionHash, 10*time.Second, true)
		require.NoError(t, err)
	}

	var faucetAmount = strconv.FormatInt(protocol.AcmeFaucetAmount*protocol.AcmePrecision, 10)
	for i := range liteAccounts {
		//now query the account to make sure each account has 10 acme.
		commandLine := fmt.Sprintf("account get %s", liteAccounts[i])
		r, err := tc.execute(t, commandLine)
		require.NoError(t, err)

		res := new(client.ChainQueryResponse)
		acc := new(protocol.LiteTokenAccount)
		res.Data = acc
		require.NoError(t, json.Unmarshal([]byte(r), &res))

		if !beenFauceted[i] {
			require.Equal(t, faucetAmount, acc.Balance.String(),
				"balance does not match not expected for account %s", liteAccounts[i])
		}
	}
}
