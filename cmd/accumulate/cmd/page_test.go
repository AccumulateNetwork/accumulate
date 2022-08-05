package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func init() {
	testMatrix.addTest(testCase4_1)
	testMatrix.addTest(testCase4_2)
	testMatrix.addTest(testCase4_3)
	testMatrix.addTest(testCase4_4)
	testMatrix.addTest(testCase4_5)
	testMatrix.addTest(testCase4_6)
	testMatrix.addTest(testCase4_7)
	testMatrix.addTest(testCase4_8)

}

//testCase4_1 Create a key book with a page using the same key as the book
func testCase4_1(t *testing.T, tc *testCmd) {

	r, err := tc.executeTx(t, "book create acc://RedWagon.acme red1 acc://RedWagon.acme/book4_1")
	require.NoError(t, err)
	t.Log(r)
}

//testCase4_2 Create a key book with a page with key red2
func testCase4_2(t *testing.T, tc *testCmd) {

	r, err := tc.executeTx(t, "book create acc://RedWagon.acme red1 acc://RedWagon.acme/book0 red2")
	require.NoError(t, err)
	t.Log(r)
}

//testCase4_3 Add a key to a key page
func testCase4_3(t *testing.T, tc *testCmd) {

	_, err := tc.executeTx(t, "credits %s acc://RedWagon.acme/book0/1 1000 10", liteAccounts[2])
	require.NoError(t, err)

	r, err := tc.executeTx(t, "page key add acc://RedWagon.acme/book0/1 red2 red4")
	require.NoError(t, err)
	t.Log(r)

	r, err = tc.executeTx(t, "page key add acc://RedWagon.acme/book0/1 red2 red5")
	require.NoError(t, err)
	t.Log(r)
}

// accumulate page create [origin adi url] [key name[@key book or page]] [public key 1] ... [public key hex or name n + 1] Create new key page with 1 to N+1 public keys
//testCase4_4 Create additional key pages sponsored by a book
func testCase4_4(t *testing.T, tc *testCmd) {

	r, err := tc.executeTx(t, "page create acc://RedWagon.acme/book0 red2 red3")
	require.NoError(t, err)
	t.Log(r)

	r, err = tc.executeTx(t, "page create acc://RedWagon.acme/book0 red2 red5")
	require.NoError(t, err)
	t.Log(r)

}

//testCase4_5 Create an adi token account bound to a key book
func testCase4_5(t *testing.T, tc *testCmd) {

	r, err := tc.executeTx(t, "account create token acc://RedWagon.acme red1 acc://RedWagon.acme/acct2 acc://ACME acc://RedWagon.acme/book0")
	require.NoError(t, err)

	t.Log(r)
}

//testCase4_6 Delete a key in a key page
func testCase4_6(t *testing.T, tc *testCmd) {

	//remove red5
	r, err := tc.executeTx(t, "page key remove acc://RedWagon.acme/book0/1 red2 red5")
	require.NoError(t, err)

	//remove red4
	r, err = tc.executeTx(t, "page key remove acc://RedWagon.acme/book0/1 red2 red4")
	require.NoError(t, err)

	//remove red2
	r, err = tc.executeTx(t, "page key remove acc://RedWagon.acme/book0/1 red2 red2")
	require.EqualError(t, err, "cannot delete last key of the highest priority page of a key book")

	t.Log(r)
}

//testCase4_7 update a key in a key page
func testCase4_7(t *testing.T, tc *testCmd) {

	//replace key3 with key 4
	r, err := tc.executeTx(t, "page key update acc://RedWagon.acme/book0/1 red2 red2 red5")
	require.NoError(t, err)

	t.Log(r)
}

//testCase4_8 Sign a transaction with a secondary key page
func testCase4_8(t *testing.T, tc *testCmd) {

	t.Log("Skipping test to await for full support for v2")

	//commandLine := fmt.Sprintf("tx create %s acc://RedWagon.acme/acct2 5", liteAccounts[0])
	//r, err := tc.execute(t, commandLine)
	//require.NoError(t, err)
	//
	//time.Sleep(2 * time.Second)
	//commandLine = fmt.Sprintf("tx create acc://RedWagon.acme/acct2 red3 1 1 acc://Redwagon.acme/acct 1.1234")
	//r, err = tc.execute(t, commandLine)
	//require.NoError(t, err)
	//
	//t.Log(r)
	//
	//time.Sleep(2 * time.Second)
}
