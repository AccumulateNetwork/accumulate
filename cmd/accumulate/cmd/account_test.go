package cmd

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	testMatrix.addTest(testCase1_1)
	testMatrix.addTest(testCase1_2)
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

	r, err := tc.executeTx(t, "account create token acc://RedWagon red1 acc://RedWagon/acct acc://acme acc://RedWagon/book")
	require.NoError(t, err)

	t.Log(r)

}

//unitTest3_2
//Create ADI Token Account without parent ADI, should fail
func testCase3_2(t *testing.T, tc *testCmd) {
	t.Helper()

	r, err := tc.execute(t, "account create token acc://RedWagon red1 acmeacct2 acc://acme acc://RedWagon/book")
	require.Error(t, err)

	t.Log(r)

}

//unitTest3_3
//Create ADI Token Account with invalid token URL, should fail
func testCase3_3(t *testing.T, tc *testCmd) {
	t.Helper()

	r, err := tc.execute(t, "account create token acc://RedWagon red1 acc://RedWagon/acmeacct acc://factoid acc://RedWagon/book")
	require.Error(t, err)

	t.Log(r)

}

//unitTest1_2
//Create Lite Token Accounts based on RCD1-based factoid addresses
func testCase1_2(t *testing.T, tc *testCmd) {
	t.Helper()

	fs := "Fs1jQGc9GJjyWNroLPq7x6LbYQHveyjWNPXSqAvCEKpETNoTU5dP"
	fa := "FA22de5NSG2FA2HmMaD4h8qSAZAJyztmmnwgLPghCQKoSekwYYct"

	//quick check to make sure the factoid addresses are correct.
	fa2, rcdHash, _, err := protocol.GetFactoidAddressRcdHashPkeyFromPrivateFs(fs)
	require.NoError(t, err)
	_ = rcdHash
	require.Equal(t, fa, fa2)

	//quick protocol import check.
	r, err := tc.execute(t, "key import factoid "+fs)
	require.NoError(t, err)
	kr := KeyResponse{}
	require.NoError(t, json.Unmarshal([]byte(r), &kr))

	// make sure the right rcd account exists and the label is a FA address
	lt, err := protocol.GetLiteAccountFromFactoidAddress(fa)
	require.NoError(t, err)
	require.Equal(t, lt.String(), kr.LiteAccount.String())
	require.Equal(t, *kr.Label.AsString(), fa)

	//now faucet the rcd account
	_, err = tc.executeTx(t, "faucet "+kr.LiteAccount.String())
	require.NoError(t, err)

	//now make sure rcd account has the funds
	bal, err := testGetBalance(t, tc, kr.LiteAccount.String())
	require.NoError(t, err)
	require.Equal(t, bal, "200000000000000")

	_, err = tc.execute(t, "get "+kr.LiteAccount.String())
	require.NoError(t, err)

	_, err = tc.executeTx(t, "credits "+kr.LiteAccount.String()+" "+kr.LiteAccount.RootIdentity().String()+" 100")
	require.NoError(t, err)

	legacyAccount := KeyResponse{}
	r, err = tc.execute(t, "account generate")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(r), &legacyAccount))

	//now transfer from an RCD based account to an ED25519 based account
	_, err = tc.executeTx(t, "tx create "+kr.LiteAccount.String()+" "+legacyAccount.LiteAccount.String()+" "+"100.00")
	require.NoError(t, err)

	//now make sure it transferred
	bal, err = testGetBalance(t, tc, legacyAccount.LiteAccount.String())
	require.NoError(t, err)
	require.Equal(t, bal, "10000000000")
}

//testGetBalance helper function to get the balance of a token account
func testGetBalance(t *testing.T, tc *testCmd, accountUrl string) (string, error) {
	//now query the account to make sure each account has 10 acme.
	commandLine := fmt.Sprintf("account get %s", accountUrl)
	r, err := tc.execute(t, commandLine)
	if err != nil {
		return "", err
	}

	res := new(api.ChainQueryResponse)
	acc := new(protocol.LiteTokenAccount)
	res.Data = acc
	err = json.Unmarshal([]byte(r), &res)
	if err != nil {
		return "", err
	}

	return acc.Balance.String(), nil
}
