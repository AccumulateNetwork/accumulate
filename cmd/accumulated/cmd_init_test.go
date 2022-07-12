package main

import (
	"bytes"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	proxy_testing "gitlab.com/accumulatenetwork/accumulate/pkg/proxy/testing"
	"strings"
	"testing"
)

func TestInitSeeds(t *testing.T) {
	proxyClient, accClient := proxy_testing.LaunchFakeProxy(t)
	_ = proxyClient
	_ = accClient
	var args []string
	workDir := t.TempDir()
	commandLine := fmt.Sprintf("accumulated init node 0 --listen tcp://0.0.0.0:%d --work-dir %s --seed %s", proxy_testing.Partitions[1].BasePort, workDir, proxy_testing.Endpoint)
	args = strings.Split(commandLine, " ")

	e := bytes.NewBufferString("")
	b := bytes.NewBufferString("")
	cmd := new(cobra.Command)
	cmd.SetErr(e)
	cmd.SetOut(b)

	cmd.AddCommand(cmdMain)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())
}
