package node_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	cfg "github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/node"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmnet "github.com/tendermint/tendermint/libs/net"
)

func TestNodeLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	opts, err := acctesting.NodeInitOptsForNetwork(acctesting.LocalBVN)
	require.NoError(t, err)
	opts.WorkDir = t.TempDir()
	opts.Port = getFreePort(t)
	opts.Config[0].Accumulate.Networks[0] = fmt.Sprintf("tcp://%s:%d", opts.RemoteIP[0], opts.Port+networks.TmRpcPortOffset)

	require.NoError(t, node.Init(opts)) // Configure

	nodeDir := filepath.Join(opts.WorkDir, "Node0") //
	//disable web site
	c, err := cfg.Load(nodeDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	c.Accumulate.WebsiteEnabled = false
	cfg.Store(c)

	node, db, _, err := acctesting.NewBVNN(acctesting.BVNNOptions{
		Dir:       nodeDir,
		LogWriter: logging.TestLogWriter(t),
	}, t.Cleanup)
	require.NoError(t, err)          // Initialize the node
	require.NoError(t, node.Start()) // Start the node
	require.NoError(t, node.Stop())  // Stop the node
	node.Wait()                      // Wait for the node to stop

	// procfs is a linux thing
	if runtime.GOOS != "linux" {
		return
	}

	// Ensure valacc.db is closed
	db.GetDB().Close()

	fds := filepath.Join("/proc", fmt.Sprint(os.Getpid()), "fd")
	entries, err := os.ReadDir(fds)
	require.NoError(t, err)
	for _, e := range entries {
		if e.Type()&os.ModeSymlink == 0 {
			continue
		}

		file, err := filepath.EvalSymlinks(filepath.Join(fds, e.Name()))
		if err != nil {
			continue
		}

		rel, err := filepath.Rel(nodeDir, file)
		require.NoError(t, err)

		if strings.HasPrefix(rel, "../") {
			continue
		}

		assert.Failf(t, "Files are still open after the node was shut down", "%q is open", rel)
	}
}

func TestNodeSetupTwiceWithPrometheus(t *testing.T) {
	t.Skip("https://github.com/tendermint/tendermint/issues/7076")

	for i := 0; i < 2; i++ {
		t.Run(fmt.Sprintf("Try %d", i+1), func(t *testing.T) {
			opts, err := acctesting.NodeInitOptsForNetwork(acctesting.LocalBVN)
			require.NoError(t, err)
			opts.ShardName = "accumulate"
			opts.WorkDir = t.TempDir()
			opts.Port = getFreePort(t)
			opts.Config[0].Instrumentation.Prometheus = true
			opts.Config[0].Accumulate.Networks[0] = fmt.Sprintf("tcp://%s:%d", opts.RemoteIP[0], opts.Port+networks.TmRpcPortOffset)

			opts2 := acctesting.BVNNOptions{
				Dir:       filepath.Join(opts.WorkDir, "Node0"),
				LogWriter: logging.TestLogWriter(t),
			}
			require.NoError(t, node.Init(opts))                     // Configure
			node, _, _, err := acctesting.NewBVNN(opts2, t.Cleanup) // Initialize
			require.NoError(t, err)                                 //
			require.NoError(t, node.Start())                        // Start
			require.NoError(t, node.Stop())                         // Stop
			node.Wait()                                             //
		})
	}
}

func getFreePort(t *testing.T) int {
	t.Helper()
	port, err := tmnet.GetFreePort()
	require.NoError(t, err)
	return port
}
