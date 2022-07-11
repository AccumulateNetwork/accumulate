package main

import (
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	proxy_testing "gitlab.com/accumulatenetwork/accumulate/pkg/proxy/testing"
	"testing"
)

func TestInitSeeds(t *testing.T) {
	proxyClient, accClient := proxy_testing.LaunchFakeProxy(t)
	_ = proxyClient
	_ = accClient
	var args []string
	args = append(args, "0")
	args = append(args, "--work-dir")
	args = append(args, t.TempDir())
	args = append(args, "--seed-proxy")
	args = append(args, proxy_testing.Endpoint)
	cmd := new(cobra.Command)
	cmd.AddCommand(cmdInit)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())
}
