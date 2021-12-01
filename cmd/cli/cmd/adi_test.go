package cmd

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/stretchr/testify/require"
)

func init() {
	testMatrix.addTest(testCase2_1)
	testMatrix.addTest(testCase2_2)
	testMatrix.addTest(testCase2_3)
	testMatrix.addTest(testCase2_4)
	testMatrix.addTest(testCase2_5)
	testMatrix.addTest(testCase2_6)
	testMatrix.addTest(testCase2_7)
}

//testCase2_1
//Create an ADI "RedWagon" sponsored by a Lite Account
func testCase2_1(t *testing.T, tc *testCmd) {
	t.Helper()

	//create 5 addresses
	for i := 1; i <= 5; i++ {
		_, err := tc.execute(t, fmt.Sprintf("key generate red%d", i))
		require.NoError(t, err)
	}

	//faucet the lite account to make sure there are tokens available
	testCase5_1(t, tc)

	//need to wait a sec to make sure faucet tx settles
	time.Sleep(2 * time.Second)

	commandLine := fmt.Sprintf("adi create %s acc://RedWagon red1", liteAccounts[0])
	_, err := tc.execute(t, commandLine)
	require.NoError(t, err)

	//need to wait 2 secs to make sure adi create settles
	time.Sleep(2 * time.Second)

	//if this doesn't fail, then adi is created
	_, err = tc.execute(t, "adi directory acc://RedWagon")
	require.NoError(t, err)

}

//testCase2_2
//Create an ADI with a "." contained within the name, expect failure
func testCase2_2(t *testing.T, tc *testCmd) {
	t.Helper()

	commandLine := fmt.Sprintf("adi create %s acc://Red.Wagon red1", liteAccounts[0])
	_, err := tc.execute(t, commandLine)
	require.Error(t, err)
}

//testCase2_3
//Create an ADI with invalid UTF-8, expect failure
func testCase2_3(t *testing.T, tc *testCmd) {
	t.Helper()

	commandLine := fmt.Sprintf("adi create %s acc://%c%c%c%c%c%c%c%c%c%c%c%c%c red1", liteAccounts[0],
		0xC0, 0xC1, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xFF)
	_, err := tc.execute(t, commandLine)
	require.Error(t, err)
}

//testCase2_4
//Create an ADI with a number only, expect failure
func testCase2_4(t *testing.T, tc *testCmd) {
	t.Helper()
	commandLine := fmt.Sprintf("adi create %s acc://12345 red1", liteAccounts[0])
	_, err := tc.execute(t, commandLine)
	require.Error(t, err)

}

//testCase2_5
//Create an ADI that already exists
func testCase2_5(t *testing.T, tc *testCmd) {
	t.Helper()

	t.Log("Need to support get txid with upgrade to V2 api to perform test, skipping... ")
	return

	//uncomment after V2 upgrade
	//commandLine := fmt.Sprintf("adi create %s acc://RedWagon red5 blue green", liteAccounts[0])
	//r, err := tc.execute(t, commandLine)
	//require.NoError(t, err)
	//
	//var res map[string]interface{}
	//require.NoError(t, json.Unmarshal([]byte(r), &res))
	//
	//time.Sleep(time.Second)
	////now query the txid
	//commandLine = fmt.Sprintf("get txid %v", res["txid"])
	//r, err = tc.execute(t, commandLine)
	//t.Log(r)
}

//testCase2_6
//Create an ADI from another ADI
func testCase2_6(t *testing.T, tc *testCmd) {
	t.Helper()

	commandLine := fmt.Sprintf("adi create acc://RedWagon red1 acc://Redstone red2")
	r, err := tc.execute(t, commandLine)
	require.NoError(t, err)

	t.Log(r)
	//need to wait 2 secs to make sure adi create settles
	time.Sleep(2 * time.Second)

	//if this doesn't fail, then adi is created
	r, err = tc.execute(t, "adi directory acc://Redstone")
	t.Log(r)
	require.NoError(t, err)
}

//testCase2_7
//Create an ADI with the same encoding as Lite address, should fail
func testCase2_7(t *testing.T, tc *testCmd) {
	t.Helper()
	k, err := tc.execute(t, "key export private red2")
	require.NoError(t, err)
	var res map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(k), &res))

	pubKey, err := pubKeyFromString(fmt.Sprintf("%s", res["publicKey"]))
	require.NoError(t, err)
	u, err := protocol.AnonymousAddress(pubKey, "acc://ACME")
	require.NoError(t, err)

	commandLine := fmt.Sprintf("adi create acc://RedWagon red1 %s red2", u.String())
	_, err = tc.execute(t, commandLine)
	require.Error(t, err)
}
