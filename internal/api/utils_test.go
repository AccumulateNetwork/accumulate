package api_test

import (
	"path/filepath"
	"testing"

	"github.com/AccumulateNetwork/accumulate/internal/accumulated"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/node"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/stretchr/testify/require"
)

func startBVC(t *testing.T, dir string) *accumulated.Daemon {
	t.Helper()

	// Configure
	opts := acctesting.NodeInitOptsForNetwork(acctesting.LocalBVN)
	opts.WorkDir = dir
	opts.Config[0].Accumulate.API.EnableSubscribeTX = true
	require.NoError(t, node.Init(opts))

	// Start
	daemon, err := acctesting.RunDaemon(acctesting.DaemonOptions{
		Dir:       filepath.Join(dir, "Node0"),
		LogWriter: logging.TestLogWriter(t),
	}, t.Cleanup)
	require.NoError(t, err)
	return daemon
}
