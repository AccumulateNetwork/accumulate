package cmd

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	api2 "gitlab.com/accumulatenetwork/accumulate/types/api"
)

func init() {
	testMatrix.addTest(testCase1_1)
	testMatrix.addTest(testCase3_1)
	testMatrix.addTest(testCase3_2)
	testMatrix.addTest(testCase3_3)
}

//testCase1_1 Generate 100 lite account addresses in cli
func testCase1_1(t *testing.T, tc *testCmd) {
	for i := 0; i < 100; i++ {
		r, err := tc.execute(t, "account generate")
		require.NoError(t, err)
		var out map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(r), &out))
		if _, ok := out["name"]; !ok {
			t.Fatalf("malformed json, expecting field \"name\"\n")
		}
		l, _ := LabelForLiteTokenAccount(liteAccounts[i])
		if out["name"] != l {
			t.Fatalf("account generate error, expected %s, but got %s", liteAccounts[i], out["name"])
		}
	}
}

//unitTest3_1
//Create ADI Token Account (URL), should pass
func testCase3_1(t *testing.T, tc *testCmd) {
	t.Helper()

	commandLine := fmt.Sprintf("account create token acc://RedWagon red1 acc://RedWagon/acct acc://acme acc://RedWagon/book0")
	r, err := tc.executeTx(t, commandLine)
	require.NoError(t, err)

	t.Log(r)

}

//unitTest3_2
//Create ADI Token Account without parent ADI, should fail
func testCase3_2(t *testing.T, tc *testCmd) {
	t.Helper()

	commandLine := fmt.Sprintf("account create token acc://RedWagon red1 acmeacct2 acc://acme acc://RedWagon/book0")
	r, err := tc.execute(t, commandLine)
	require.Error(t, err)

	t.Log(r)

}

//unitTest3_3
//Create ADI Token Account with invalid token URL, should fail
func testCase3_3(t *testing.T, tc *testCmd) {
	t.Helper()

	commandLine := fmt.Sprintf("account create token acc://RedWagon red1 acc://RedWagon/acmeacct acc://factoid acc://RedWagon/book0")
	r, err := tc.execute(t, commandLine)
	require.Error(t, err)

	t.Log(r)

}

//testGetBalance helper function to get the balance of a token account
func testGetBalance(t *testing.T, tc *testCmd, accountUrl string) (string, error) {
	//now query the account to make sure each account has 10 acme.
	commandLine := fmt.Sprintf("account get %s", accountUrl)
	r, err := tc.execute(t, commandLine)
	if err != nil {
		return "", err
	}

	res := api2.APIDataResponse{}
	err = json.Unmarshal([]byte(r), &res)
	if err != nil {
		return "", err
	}

	acc := protocol.LiteTokenAccount{}
	err = json.Unmarshal(*res.Data, &acc)
	if err != nil {
		return "", err
	}
	return acc.Balance.String(), nil
}
