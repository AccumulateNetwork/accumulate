// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package cmd

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	testMatrix.addTest(testCase2_1)
	testMatrix.addTest(testCase2_2)
	testMatrix.addTest(testCase2_3)
	testMatrix.addTest(testCase2_4)
	testMatrix.addTest(testCase2_5)
	testMatrix.addTest(testCase2_6a)
	testMatrix.addTest(testCase2_6b)
	testMatrix.addTest(testCase2_7a)
	testMatrix.addTest(testCase2_7b)
}

//testCase2_1
//Create an ADI "RedWagon" sponsored by a Lite Account
func testCase2_1(t *testing.T, tc *testCmd) {

	//create 5 addresses
	for i := 1; i <= 5; i++ {
		_, err := tc.execute(t, fmt.Sprintf("key generate red%d", i))
		require.NoError(t, err)
	}

	//faucet the lite account to make sure there are tokens available
	testCase5_1(t, tc)

	u, err := url.Parse(liteAccounts[0])
	require.NoError(t, err)
	liteId := u.RootIdentity().String()

	_, err = tc.executeTx(t, "credits %s %s 1000 100 0.0", liteAccounts[0], liteId)
	require.NoError(t, err)

	_, err = tc.executeTx(t, "adi create %s acc://RedWagon.acme red1", liteId)
	require.NoError(t, err)

	//if this doesn't fail, then adi is created
	_, err = tc.execute(t, "adi directory acc://RedWagon.acme 0 10")
	require.NoError(t, err)

}

//testCase2_2
//Create an ADI with a "." contained within the name, expect failure
func testCase2_2(t *testing.T, tc *testCmd) {

	commandLine := fmt.Sprintf("adi create %s acc://Red.Wagon red1", liteAccounts[0])
	_, err := tc.execute(t, commandLine)
	require.Error(t, err)
}

//testCase2_3
//Create an ADI with invalid UTF-8, expect failure
func testCase2_3(t *testing.T, tc *testCmd) {

	commandLine := fmt.Sprintf("adi create %s acc://%c%c%c%c%c%c%c%c%c%c%c%c%c red1", liteAccounts[0],
		0xC0, 0xC1, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xFF)
	_, err := tc.execute(t, commandLine)
	require.Error(t, err)
}

//testCase2_4
//Create an ADI with a number only, expect failure
func testCase2_4(t *testing.T, tc *testCmd) {
	commandLine := fmt.Sprintf("adi create %s acc://12345.acme red1", liteAccounts[0])
	_, err := tc.execute(t, commandLine)
	require.Error(t, err)

}

//testCase2_5
//Create an ADI that already exists
func testCase2_5(t *testing.T, tc *testCmd) {

	t.Log("Need to support get txid with upgrade to V2 api to perform test, skipping... ")

	//uncomment after V2 upgrade
	//commandLine := fmt.Sprintf("adi create %s acc://RedWagon.acme red5 blue green", liteAccounts[0])
	//r, err := tc.executeTx(t, commandLine)
	//require.NoError(t, err)
	//
	//var res map[string]interface{}
	//require.NoError(t, json.Unmarshal([]byte(r), &res))
	//
	////now query the txid
	//commandLine = fmt.Sprintf("get txid %v", res["txid"])
	//r, err = tc.execute(t, commandLine)
	//t.Log(r)
}

//testCase2_6a
//Create an ADI from another ADI
func testCase2_6a(t *testing.T, tc *testCmd) {

	//attempt to add 1000 credits with only 9 acme with 15% slippage
	_, err := tc.executeTx(t, "credits %s acc://RedWagon.acme/book/1 1000 10", liteAccounts[1])
	require.NoError(t, err)

	r, err := tc.executeTx(t, "adi create acc://RedWagon.acme red1 acc://Redstone.acme red2")
	require.NoError(t, err)

	t.Log(r)

	//if this doesn't fail, then adi is created
	r, err = tc.execute(t, "adi directory acc://Redstone.acme 0 10")
	t.Log(r)
	require.NoError(t, err)
}

//testCase2_6b
//Create an ADI with the same encoding as Lite address, should fail
func testCase2_6b(t *testing.T, tc *testCmd) {
	k, err := tc.execute(t, "key export private red2")
	require.NoError(t, err)
	var res map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(k), &res))

	pubKey, err := pubKeyFromString(fmt.Sprintf("%s", res["publicKey"]))
	require.NoError(t, err)
	u, err := protocol.LiteTokenAddress(pubKey.PublicKey, protocol.ACME, protocol.SignatureTypeED25519)
	require.NoError(t, err)

	commandLine := fmt.Sprintf("adi create acc://RedWagon.acme red1 %s red2", u.String())
	_, err = tc.execute(t, commandLine)
	require.Error(t, err)
}

//testCase2_7a
//Create sub-ADIs
func testCase2_7a(t *testing.T, tc *testCmd) {

	_, err := tc.executeTx(t, "credits %s acc://RedWagon.acme/book/1 1000 10", liteAccounts[1])
	require.NoError(t, err)

	r, err := tc.executeTx(t, "adi create acc://RedWagon.acme red1 acc://RedWagon.acme/sub1 red2")
	t.Log(r)
	require.NoError(t, err)

	//if this doesn't fail, then adi is created
	r, err = tc.execute(t, "adi directory acc://RedWagon.acme/sub1 0 10")
	require.NoError(t, err)
	t.Log(r)
}

//testCase2_7a
//Create sub-ADIs with the wrong parents, should fail
func testCase2_7b(t *testing.T, tc *testCmd) {

	_, err := tc.executeTx(t, "credits %s acc://RedWagon.acme/book/1 1000 10", liteAccounts[1])
	require.NoError(t, err)

	_, err = tc.executeTx(t, "adi create acc://RedWagon.acme red1 acc://RedWagon.acme/sub1/sub2 red2")
	require.Error(t, err)
}
