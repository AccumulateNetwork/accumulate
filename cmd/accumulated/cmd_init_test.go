package main

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/config"
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
		fmt.Sprintf("accumulated init node %s --work-dir %s --listen=http://127.11.11.11:%s --no-prometheus", dnEndpoint.String(), workDirs[0], dnEndpoint.Port()),
		fmt.Sprintf("accumulated init node %s --work-dir %s --listen=http://127.11.11.11:%s --no-prometheus", bvnEndpoint.String(), workDirs[1], bvnEndpoint.Port()),
		fmt.Sprintf("accumulated init node directory.devnet --seed %s --work-dir %s --no-prometheus", proxy_testing.Endpoint, workDirs[2]),
		fmt.Sprintf("accumulated init node bvn1.devnet --seed %s --work-dir %s --no-prometheus", proxy_testing.Endpoint, workDirs[3]),
		fmt.Sprintf("accumulated init dual %s --work-dir %s --public=http://127.11.11.11 --listen=tcp://127.11.11.12 --no-prometheus", bvnEndpoint.String(), workDirs[4]),
	}

	e := bytes.NewBufferString("")
	b := bytes.NewBufferString("")
	cmd := new(cobra.Command)
	cmd.SetErr(e)
	cmd.SetOut(b)

	cmd.AddCommand(cmdMain)

	for i, cl := range commandLine {
		initInitFlags()
		args = strings.Split(cl, " ")
		cmd.SetArgs(args)
		require.NoError(t, cmd.Execute())
		require.NoError(t, DidError, "when executing: ", cl)

		//fix the timeouts to match devnet bvn to avoid consensus error
		c, err := config.Load(workDirs[i] + "/bvnn")
		if err != nil {
			c, err = config.Load(workDirs[i] + "/dnn")
			if err != nil {
				continue
			}
		}
		//handle the case for the dual node bvnn, don't care about other error
		c.Consensus.TimeoutCommit = time.Millisecond * 200
		require.NoError(t, config.Store(c))
	}

	//now for kicks fire up a dual node
	runNodes := []string{
		fmt.Sprintf("accumulated run -w %s/dnn --ci-stop-after 5s", workDirs[0]),
		fmt.Sprintf("accumulated run -w %s/bvnn --ci-stop-after 5s", workDirs[1]),
		fmt.Sprintf("accumulated run -w %s/dnn --ci-stop-after 5s", workDirs[2]),
		fmt.Sprintf("accumulated run -w %s/bvnn --ci-stop-after 5s", workDirs[3]),
		fmt.Sprintf("accumulated run-dual %s/dnn %s/bvnn --ci-stop-after 5s", workDirs[4], workDirs[4]),
	}

	//this test will fire up various configurations to ensure things were configured ok and can start.
	//This isn't perfect, but it is a great debugging tool
	for _, run := range runNodes {
		initInitFlags()
		initRunFlags(cmd, true)

		args := strings.Split(run, " ")
		cmd.SetArgs(args)
		require.NoError(t, cmd.Execute())
		//make sure node can fire up without error
		require.NoError(t, DidError, "when executing: %s", run)

		errPrint, err := io.ReadAll(e)
		require.NoError(t, err)
		if len(errPrint) != 0 {
			t.Fatalf("%s", string(errPrint))
		}
	}
}
