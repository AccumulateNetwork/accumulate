package cmd

import (
	"encoding/json"
	"fmt"
	"testing"

	api2 "github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/AccumulateNetwork/accumulate/types/api/response"
	"github.com/stretchr/testify/require"
)

func init() {
	testMatrix.addTest(testCase5_1)
}

func testCase5_1(t *testing.T, tc *testCmd) {
	t.Helper()

	var beenFauceted []bool
	//test to see if things have already been fauceted...
	for i := range liteAccounts {
		bal, _ := testGetBalance(t, tc, liteAccounts[i])
		beenFauceted = append(beenFauceted, bal == "1000000000")
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

		acc := response.AnonTokenAccount{} //protocol.AnonTokenAccount{}
		require.NoError(t, json.Unmarshal(*res.Data, &acc), "received error on liteAccount[%d] %s ", i, liteAccounts[i])

		if !beenFauceted[i] {
			require.Equal(t, "1000000000", acc.Balance.String(),
				"balance does not match not expected for account %s", liteAccounts[i])
		}
	}
}
