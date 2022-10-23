// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package cmd

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd"
	"gitlab.com/accumulatenetwork/accumulate/config"
	core "gitlab.com/accumulatenetwork/accumulate/pkg/exp"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

type testCase func(t *testing.T, tc *testCmd)
type testMatrixTests []testCase

var testMatrix testMatrixTests

func bootstrap(t *testing.T, tc *testCmd) {

	_, err := executeCmd(tc.rootCmd,
		[]string{"-j", "-s", fmt.Sprintf("%s/v2", tc.jsonRpcAddr), "wallet", "init", "import", "mnemonic"},
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
		fmt.Sprintf("%v\n", hex.EncodeToString(tc.privKey)))
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
	tc := &testCmd{}
	tc.initalize(t)

	bootstrap(t, tc)
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
	privKey     []byte
}

// NewTestBVNN creates a BVN test Node and returns the rest and jsonrpc ports and the DN private key
func NewTestBVNN(t *testing.T) (string, []byte) {
	t.Helper()

	const valCount = 1
	const basePort = 12345
	net := simulator.SimpleNetwork("Simulator", 1, valCount)
	for i, bvn := range net.Bvns {
		for j, node := range bvn.Nodes {
			node.AdvertizeAddress = fmt.Sprintf("127.0.1.%d", 1+i*valCount+j)
			node.BasePort = basePort
		}
	}

	// Disable the sliding fee schedule
	values := new(core.GlobalValues)
	values.Globals = new(protocol.NetworkGlobals)
	values.Globals.FeeSchedule = new(protocol.FeeSchedule)

	// Initialize
	sim := NewSim(t,
		simulator.MemoryDatabase,
		net,
		simulator.GenesisWith(GenesisTime, values),
	)

	// Serve
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = sim.ListenAndServe(ctx, nil) }()

	// Step at 100 Hz
	tick := time.NewTicker(time.Second / 100)
	t.Cleanup(tick.Stop)
	go func() {
		for range tick.C {
			sim.Step()
		}
	}()

	port := basePort + config.PortOffsetDirectory + config.PortOffsetAccumulateApi
	return fmt.Sprintf("http://127.0.1.1:%d", port), sim.SignWithNode(protocol.Directory, 0).Key()
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
