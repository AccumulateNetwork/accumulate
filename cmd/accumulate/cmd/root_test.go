package cmd

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd"
	"gitlab.com/accumulatenetwork/accumulate/internal/genesis"
	"gitlab.com/accumulatenetwork/accumulate/internal/testdata"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() { acctesting.EnableDebugFeatures() }

type testCase func(t *testing.T, tc *testCmd)
type testMatrixTests []testCase

var testMatrix testMatrixTests

func bootstrap(t *testing.T, tc *testCmd) {

	_, err := executeCmd(tc.rootCmd,
		[]string{"-j", "-s", fmt.Sprintf("%s/v2", tc.jsonRpcAddr), "wallet", "init", "import"},
		"yellow yellow yellow yellow yellow yellow yellow yellow yellow yellow yellow yellow\n")
	require.NoError(t, err)

	// import eth private key.
	// res, err := tc.execute(t, "key import private 26b9b10aec1e75e68709689b446196a5235b26bb9d4c0fc91eaccc7d8b66ec16 ethKey --sigtype eth")
	res, err := executeCmd(tc.rootCmd,
		[]string{"-j", "-s", fmt.Sprintf("%s/v2", tc.jsonRpcAddr), "key", "import", "private", "ethKey", "--sigtype", "eth"},
		"26b9b10aec1e75e68709689b446196a5235b26bb9d4c0fc91eaccc7d8b66ec16\n")
	require.NoError(t, err)
	var keyResponse KeyResponse
	err = json.Unmarshal([]byte(strings.Split(res, ": ")[1]), &keyResponse)
	require.NoError(t, err)

	//add the DN private key to our key list.
	_, err = executeCmd(tc.rootCmd,
		[]string{"-j", "-s", fmt.Sprintf("%s/v2", tc.jsonRpcAddr), "key", "import", "private", "dnkey", "--sigtype", "ed25519"},
		fmt.Sprintf("%v\n", hex.EncodeToString(tc.privKey.Bytes())))
	require.NoError(t, err)

	oracle := new(protocol.AcmeOracle)
	oracle.Price = 10_000 * protocol.AcmeOraclePrecision
	data, err := oracle.MarshalBinary()
	require.NoError(t, err)

	//set the oracle price to $10,000
	resp, err := tc.executeTx(t, "data write --write-state --wait 10s dn.acme/oracle dnkey %x", data)
	require.NoError(t, err)
	ar := new(ActionResponse)
	require.NoError(t, json.Unmarshal([]byte(resp), ar))
	for _, r := range ar.Flow {
		if r.Status.Error != nil {
			require.NoError(t, r.Status.Error)
		}
	}
}

func TestCli(t *testing.T) {
	acctesting.SkipLong(t)
	acctesting.SkipPlatformCI(t, "darwin", "flaky")

	tc := &testCmd{}
	tc.initalize(t)

	bootstrap(t, tc)
	err := testFactomAddresses()
	require.NoError(t, err)
	testMatrix.execute(t, tc)

}

func GetFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

func (tm *testMatrixTests) addTest(tc testCase) {
	*tm = append(*tm, tc)

}

func (tm *testMatrixTests) execute(t *testing.T, tc *testCmd) {

	// Sort by testCase number
	sort.SliceStable(*tm, func(i, j int) bool {
		return GetFunctionName((*tm)[i]) < GetFunctionName((*tm)[j])
	})

	//execute the tests
	var skip bool
	for _, f := range testMatrix {
		name := strings.Split(GetFunctionName(f), ".")
		ok := t.Run(name[len(name)-1], func(t *testing.T) {
			if skip {
				t.SkipNow()
			}
			f(t, tc)
		})
		if !ok {
			skip = true
		}
	}
}

type testCmd struct {
	rootCmd     *cobra.Command
	jsonRpcAddr string
	privKey     crypto.PrivKey
}

//NewTestBVNN creates a BVN test Node and returns the rest and jsonrpc ports and the DN private key
func NewTestBVNN(t *testing.T) (string, crypto.PrivKey) {
	t.Helper()
	acctesting.SkipPlatformCI(t, "darwin", "requires setting up localhost aliases")

	// Start
	partitions, daemons := acctesting.CreateTestNet(t, 1, 1, 0, true)
	acctesting.RunTestNet(t, partitions, daemons)

	time.Sleep(time.Second)
	c := daemons[partitions[1]][0].Config

	return c.Accumulate.API.ListenAddress, daemons[partitions[0]][0].Key()
}

func (c *testCmd) initalize(t *testing.T) {
	t.Helper()

	walletd.InitTestDB(t)
	c.rootCmd = InitRootCmd()
	c.rootCmd.PersistentPostRun = nil

	c.jsonRpcAddr, c.privKey = NewTestBVNN(t)
	time.Sleep(2 * time.Second)
}

func (c *testCmd) execute(t *testing.T, cmdLine string) (string, error) {
	fullCommand := fmt.Sprintf("-j -s %s/v2 %s",
		c.jsonRpcAddr, cmdLine)
	args := strings.Split(fullCommand, " ")

	return executeCmd(c.rootCmd, args, "")
}

func executeCmd(cmd *cobra.Command, args []string, input string) (string, error) {
	// Reset flags
	Client = nil
	ClientTimeout = 0
	ClientDebug = false
	WantJsonOutput = false
	TxPretend = false
	Prove = false
	Memo = ""
	Metadata = ""
	SigType = ""
	Authorities = nil
	TxWait = 0
	TxNoWait = false
	TxWaitSynth = 0
	TxIgnorePending = false
	flagAccount.Lite = false

	walletd.UseUnencryptedWallet = true

	e := bytes.NewBufferString("")
	b := bytes.NewBufferString("")
	cmd.SetErr(e)
	cmd.SetOut(b)
	cmd.SetArgs(args)
	cmd.SetIn(strings.NewReader(input))
	DidError = nil
	err := cmd.Execute()
	if DidError != nil {
		return "", DidError
	}
	if err != nil {
		return "", err
	}

	errPrint, err := io.ReadAll(e)
	if err != nil {
		return "", err
	} else if len(errPrint) != 0 {
		return "", fmt.Errorf("%s", string(errPrint))
	}
	ret, err := io.ReadAll(b)
	return string(ret), err
}

func (c *testCmd) executeTx(t *testing.T, cmdLine string, args ...interface{}) (string, error) {
	cmdLine = fmt.Sprintf(cmdLine, args...)
	return c.execute(t, "--wait 10s "+cmdLine)
}

func testFactomAddresses() error {
	factomAddresses, err := genesis.LoadFactomAddressesAndBalances(strings.NewReader(testdata.FactomAddresses))
	if err != nil {
		return err
	}
	for _, address := range factomAddresses {
		res, err := GetUrl(address.Address.String())
		if err != nil {
			return err
		}
		account := res.Data.(map[string]interface{})
		balance, err := strconv.Atoi(account["balance"].(string))
		if err != nil {
			return err
		}
		if int64(balance) != 5*address.Balance {
			return fmt.Errorf("accumulate balance for fatcom address doesn't match")
		}
	}
	return nil
}
