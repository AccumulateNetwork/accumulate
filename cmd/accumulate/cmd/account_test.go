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

	r, err := tc.executeTx(t, "account create token acc://RedWagon red1 acc://RedWagon/acct acc://acme acc://RedWagon/book0")
	require.NoError(t, err)

	t.Log(r)

}

//unitTest3_2
//Create ADI Token Account without parent ADI, should fail
func testCase3_2(t *testing.T, tc *testCmd) {
	t.Helper()

	r, err := tc.execute(t, "account create token acc://RedWagon red1 acmeacct2 acc://acme acc://RedWagon/book0")
	require.Error(t, err)

	t.Log(r)

}

//unitTest3_3
//Create ADI Token Account with invalid token URL, should fail
func testCase3_3(t *testing.T, tc *testCmd) {
	t.Helper()

	r, err := tc.execute(t, "account create token acc://RedWagon red1 acc://RedWagon/acmeacct acc://factoid acc://RedWagon/book0")
	require.Error(t, err)

	t.Log(r)

}

//unitTest3_3
//Create ADI Token Account with invalid token URL, should fail
func testCase3_4(t *testing.T, tc *testCmd) {

	t.Helper()
	type FactoidPair struct {
		Fs string
		FA string
	}
	fct := []FactoidPair{
		{"Fs1jQGc9GJjyWNroLPq7x6LbYQHveyjWNPXSqAvCEKpETNoTU5dP", "FA22de5NSG2FA2HmMaD4h8qSAZAJyztmmnwgLPghCQKoSekwYYct"},
		{"Fs2wZzM2iBn4HEbhwEUZjLfcbTo5Rf6ChRNjNJWDiyWmy9zkPQNP", "FA3heCmxKCk1tCCfiAMDmX8Ctg6XTQjRRaJrF5Jagc9rbo7wqQLV"},
		{"Fs1fxJbUWQRbTXH4as6qazoZ3hunmzL9JfiEpA6diCGCBE4jauqs", "FA2PSjogJ7UWwrwtevXtoRDnpxeafuRno16pES7KY4i51pL3kWV5"},
		{"Fs2Fb4aB1N8VCBkQmeYMnLSNFtwGeRkqSmDbgw5BLa2Gdd8SVXjh", "FA3BW5bhnR6vBba42djAL9QMuKPafJurmGxM499u43PNFUdvKEKz"},
		{"Fs2sSpxAPA1YqB69rvwVhRKUkLAbtoXNjyCjoK9kCofBEbVEt9dv", "FA3sMQeEgh2z6Hr5Pr8Kfnhh49QchVpmitGswUJjc1Mw3B3BW727"}}

	for _, v := range fct {
		//quick protocol import check.
		r, err := tc.execute(t, "key import factoid "+v.Fs)
		require.Error(t, err)
		t.Log(r)
	}

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
