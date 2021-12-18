package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	net2 "net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/node"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

type testCase func(t *testing.T, tc *testCmd)
type testMatrixTests []testCase

var testMatrix testMatrixTests

func TestCli(t *testing.T) {
	acctesting.SkipLong(t)
	acctesting.SkipPlatformCI(t, "darwin", "flaky")

	tc := &testCmd{}
	tc.initalize(t)

	//set mnemonic for predictable addresses
	_, err := tc.execute(t, "key import mnemonic yellow yellow yellow yellow yellow yellow yellow yellow yellow yellow yellow yellow")
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
	for _, f := range testMatrix {
		name := strings.Split(GetFunctionName(f), ".")
		t.Run(name[len(name)-1], func(t *testing.T) { f(t, tc) })
	}
}

type testCmd struct {
	rootCmd        *cobra.Command
	directoryCmd   *exec.Cmd
	validatorCmd   *exec.Cmd
	defaultWorkDir string
	jsonRpcPort    int
}

//NewTestBVNN creates a BVN test Node and returns the rest and jsonrpc ports
func NewTestBVNN(t *testing.T, defaultWorkDir string) int {
	t.Helper()

	// Configure
	opts := acctesting.NodeInitOptsForNetwork(acctesting.LocalBVN)
	opts.WorkDir = defaultWorkDir
	require.NoError(t, node.Init(opts))

	// Start
	_, err := acctesting.RunDaemon(acctesting.DaemonOptions{
		Dir:       filepath.Join(defaultWorkDir, "Node0"),
		LogWriter: logging.TestLogWriter(t),
	}, t.Cleanup)
	require.NoError(t, err)

	time.Sleep(time.Second)
	return opts.Port + networks.AccRouterJsonPortOffset
}

func (c *testCmd) initalize(t *testing.T) {
	t.Helper()

	defaultWorkDir, err := ioutil.TempDir("", "cliTest")
	if err != nil {
		t.Fatal(err)
	}

	err = os.MkdirAll(defaultWorkDir, 0700)
	if err != nil {
		t.Fatal(err)
	}

	c.rootCmd = InitRootCmd(initDB(defaultWorkDir, true))
	c.rootCmd.PersistentPostRun = nil

	c.jsonRpcPort = NewTestBVNN(t, defaultWorkDir)
	time.Sleep(2 * time.Second)

	t.Cleanup(func() {
		os.Remove(defaultWorkDir)
	})
}

func (c *testCmd) execute(t *testing.T, cmdLine string) (string, error) {
	fullCommand := fmt.Sprintf("-j -s http://127.0.0.1:%v/v2 %s",
		c.jsonRpcPort, cmdLine)
	args := strings.Split(fullCommand, " ")

	e := bytes.NewBufferString("")
	b := bytes.NewBufferString("")
	c.rootCmd.SetErr(e)
	c.rootCmd.SetOut(b)
	c.rootCmd.SetArgs(args)
	c.rootCmd.Execute()

	errPrint, err := ioutil.ReadAll(e)
	if err != nil {
		return "", err
	} else if len(errPrint) != 0 {
		return "", fmt.Errorf("%s", string(errPrint))
	}
	ret, err := ioutil.ReadAll(b)
	return string(ret), err
}

func (c *testCmd) executeTx(t *testing.T, cmdLine string) (string, error) {
	out, err := c.execute(t, cmdLine)
	if err == nil {
		waitForTxns(t, c, out)
	}
	return out, err
}

// listenHttpUrl
// takes a string such as `http://localhost:123` and creates a TCP listener.
func listenHttpUrl(s string) (net2.Listener, bool, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, false, fmt.Errorf("invalid address: %v", err)
	}

	if u.Path != "" && u.Path != "/" {
		return nil, false, fmt.Errorf("invalid address: path is not empty")
	}

	var secure bool
	switch u.Scheme {
	case "tcp", "http":
		secure = false
	case "https":
		secure = true
	default:
		return nil, false, fmt.Errorf("invalid address: unsupported scheme %q", u.Scheme)
	}

	l, err := net2.Listen("tcp", u.Host)
	if err != nil {
		return nil, false, err
	}

	return l, secure, nil
}

func waitForTxns(t *testing.T, tc *testCmd, jsonRes string) {
	t.Helper()

	var res struct {
		Txid string
	}
	require.NoError(t, json.Unmarshal([]byte(jsonRes), &res))

	commandLine := fmt.Sprintf("tx get --wait 10s --wait-synth 10s %s", res.Txid)
	_, err := tc.execute(t, commandLine)
	require.NoError(t, err)
}
