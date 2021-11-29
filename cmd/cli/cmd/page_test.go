package cmd

import (
	"fmt"
	"testing"
	"time"

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

//testCase4_1 Create an unbounded key page
func testCase4_1(t *testing.T, tc *testCmd) {
	t.Helper()

	commandLine := fmt.Sprintf("page create acc://RedWagon red1 acc://RedWagon/page1 red2")
	r, err := tc.execute(t, commandLine)
	require.NoError(t, err)

	t.Log(r)

	commandLine = fmt.Sprintf("page create acc://RedWagon red1 acc://RedWagon/page2 red3")
	r, err = tc.execute(t, commandLine)
	require.NoError(t, err)

	t.Log(r)

	time.Sleep(3 * time.Second)
}

//testCase4_2 Create a key book from unbounded key pages
func testCase4_2(t *testing.T, tc *testCmd) {
	t.Helper()

	commandLine := fmt.Sprintf("book create acc://RedWagon red1 acc://RedWagon/book acc://RedWagon/page1 acc://RedWagon/page2")
	r, err := tc.execute(t, commandLine)
	require.NoError(t, err)

	t.Log(r)

	time.Sleep(3 * time.Second)
}

//testCase4_3 Add a key to a key page
func testCase4_3(t *testing.T, tc *testCmd) {
	t.Helper()

	t.Log("Awaiting key page update fix for api validation error, skipping... ")
	return

	//uncomment after key page fix
	//commandLine := fmt.Sprintf("-d page key add acc://RedWagon/page1 red2 red4")
	//r, err := tc.execute(t, commandLine)
	//require.NoError(t, err)
	//
	//t.Log(r)
	//
	//time.Sleep(2 * time.Second)
}

//testCase4_4 Create additional key pages sponsored by a book
func testCase4_4(t *testing.T, tc *testCmd) {
	t.Helper()

	commandLine := fmt.Sprintf("page create acc://RedWagon/book red2 acc://RedWagon/page3 red5")
	r, err := tc.execute(t, commandLine)
	require.NoError(t, err)

	t.Log(r)

	time.Sleep(2 * time.Second)
}

//testCase4_5 Create an adi token account bound to a key book
func testCase4_5(t *testing.T, tc *testCmd) {
	t.Helper()

	commandLine := fmt.Sprintf("account create acc://RedWagon red1 acc://RedWagon/acct2 acc://ACME acc://RedWagon/book")
	r, err := tc.execute(t, commandLine)
	require.NoError(t, err)

	t.Log(r)

	time.Sleep(3 * time.Second)
}

//testCase4_6 Delete a key in a key page
func testCase4_6(t *testing.T, tc *testCmd) {
	t.Helper()

	t.Log("Awaiting key page update fix for api validation error, skipping... ")
	return

	//uncomment after fix key page remove
	////remove red4
	//commandLine := fmt.Sprintf("page key remove acc://RedWagon/page2 red3 red4")
	//r, err := tc.execute(t, commandLine)
	//require.NoError(t, err)
	//
	//t.Log(r)
	//
	//time.Sleep(2 * time.Second)
}

//testCase4_7 update a key in a key page
func testCase4_7(t *testing.T, tc *testCmd) {
	t.Helper()

	t.Log("Awaiting key page update fix for api validation error, skipping... ")
	return

	//uncomment after key page update fix.
	////replace key3 with key 4
	//commandLine := fmt.Sprintf("page key update acc://RedWagon/page2 red3 red3 red4")
	//r, err := tc.execute(t, commandLine)
	//require.NoError(t, err)
	//
	//t.Log(r)
	//
	//time.Sleep(2 * time.Second)
}

//testCase4_8 Sign a transaction with a secondary key page
func testCase4_8(t *testing.T, tc *testCmd) {
	t.Helper()

	t.Log("Skipping test to await for full support for v2")
	return

	//commandLine := fmt.Sprintf("tx create %s acc://RedWagon/acct2 5", liteAccounts[0])
	//r, err := tc.execute(t, commandLine)
	//require.NoError(t, err)
	//
	//time.Sleep(4 * time.Second)
	//commandLine = fmt.Sprintf("tx create acc://RedWagon/acct2 red3 1 1 acc://Redwagon/acct 1.1234")
	//r, err = tc.execute(t, commandLine)
	//require.NoError(t, err)
	//
	//t.Log(r)
	//
	//time.Sleep(2 * time.Second)
}
