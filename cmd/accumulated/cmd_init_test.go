package main

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	proxy_testing "gitlab.com/accumulatenetwork/accumulate/pkg/proxy/testing"
)

func TestInitSeeds(t *testing.T) {
	proxyClient, accClient, dnEndpoint, bvnEndpoint := proxy_testing.LaunchFakeProxy(t)
	_ = proxyClient
	_ = accClient
	var args []string
	workDir := t.TempDir()

	workDirs := []string{
		filepath.Join(workDir, "init_node_dn_test"),
		filepath.Join(workDir, "init_node_bvn_test"),
		filepath.Join(workDir, "init_node_dn_seed_test"),
		filepath.Join(workDir, "init_node_bvn_seed_test"),
		filepath.Join(workDir, "init_dual_test"),
	}

	commandLine := []string{
		fmt.Sprintf("accumulated init node %s --work-dir %s", dnEndpoint.String(), workDirs[0]),
		fmt.Sprintf("accumulated init node %s --work-dir %s", bvnEndpoint.String(), workDirs[1]),
		fmt.Sprintf("accumulated init node directory.devnet --seed %s --work-dir %s", proxy_testing.Endpoint, workDirs[2]),
		fmt.Sprintf("accumulated init node bvn1.devnet --seed %s --work-dir %s", proxy_testing.Endpoint, workDirs[3]),
		fmt.Sprintf("accumulated init dual %s --work-dir %s", bvnEndpoint.String(), workDirs[4]),
	}

	e := bytes.NewBufferString("")
	b := bytes.NewBufferString("")
	cmd := new(cobra.Command)
	cmd.SetErr(e)
	cmd.SetOut(b)

	cmd.AddCommand(cmdMain)

	for _, cl := range commandLine {
		args = strings.Split(cl, " ")
		cmd.SetArgs(args)
		require.NoError(t, cmd.Execute())
	}

	//now for kicks fire up a dual node
	runDual := fmt.Sprintf("accumulated run-dual %s/dnn %s/bvn", workDirs[0], workDirs[1])

	args = strings.Split(runDual, " ")
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())

	errPrint, err := io.ReadAll(e)
	require.NoError(t, err)
	if len(errPrint) != 0 {
		t.Fatalf("%s", string(errPrint))
	}
	ret, err := io.ReadAll(b)
	require.NoError(t, err)
	t.Log(ret)
	//send in a transaction to the dual node
}
