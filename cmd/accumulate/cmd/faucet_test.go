package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	api2 "gitlab.com/accumulatenetwork/accumulate/types/api"
)

func init() {
	testMatrix.addTest(testCase5_1)
}

func testCase5_1(t *testing.T, tc *testCmd) {
	t.Helper()

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
		require.Equal(t, api.ErrCodeNotFound, int(jerr.Err.Code))
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
		waitForTxns(t, tc, r)
	}

	for i := range liteAccounts {
		//now query the account to make sure each account has 10 acme.
		commandLine := fmt.Sprintf("account get %s", liteAccounts[i])
		r, err := tc.execute(t, commandLine)
		require.NoError(t, err)

		res := api2.APIDataResponse{}
		require.NoError(t, json.Unmarshal([]byte(r), &res))

		acc := protocol.LiteTokenAccount{}
		require.NoError(t, json.Unmarshal(*res.Data, &acc), "received error on liteAccount[%d] %s ", i, liteAccounts[i])

		if !beenFauceted[i] {
			require.Equal(t, "10000000000", acc.Balance.String(),
				"balance does not match not expected for account %s", liteAccounts[i])
		}
	}
}
