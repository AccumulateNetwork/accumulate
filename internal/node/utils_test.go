package node_test

import (
	"fmt"
	"net"
	"path/filepath"
	"testing"

	"github.com/AccumulateNetwork/accumulate/config"
	cfg "github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/abci"
	"github.com/AccumulateNetwork/accumulate/internal/accumulated"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/node"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/stretchr/testify/require"
)

func initNodes(t *testing.T, name string, baseIP net.IP, basePort int, count int, bvnAddrs []string) []*accumulated.Daemon {
	t.Helper()

	IPs := make([]string, count)
	config := make([]*config.Config, count)

	for i := range IPs {
		ip := make(net.IP, len(baseIP))
		copy(ip, baseIP)
		ip[15] += byte(i)
		IPs[i] = ip.String()
	}

	for i := range config {
		config[i] = acctesting.DefaultConfig(cfg.BlockValidator, cfg.Validator, name)
		net := &config[i].Accumulate.Network
		if bvnAddrs == nil {
			net.BvnNames = []string{name}
		} else {
			net.BvnNames = make([]string, len(bvnAddrs))
			net.Addresses = make(map[string][]string, len(bvnAddrs))
			for i, addr := range bvnAddrs {
				bvn := fmt.Sprintf("BVN%d", i)
				net.BvnNames[i] = bvn
				net.Addresses[bvn] = []string{fmt.Sprintf("http://%s:%d", addr, basePort)}
			}
		}
	}

	workDir := t.TempDir()
	require.NoError(t, node.Init(node.InitOptions{
		WorkDir:  workDir,
		Port:     basePort,
		Config:   config,
		RemoteIP: IPs,
		ListenIP: IPs,
	}))

	daemons := make([]*accumulated.Daemon, count)
	for i := range daemons {
		daemon, err := acctesting.RunDaemon(acctesting.DaemonOptions{
			Dir:       filepath.Join(workDir, fmt.Sprintf("Node%d", i)),
			LogWriter: logging.TestLogWriter(t),
		}, t.Cleanup)
		require.NoError(t, err)
		daemon.Node_TESTONLY().ABCI.(*abci.Accumulator).OnFatal(func(err error) { require.NoError(t, err) })
		daemons[i] = daemon
	}

	return daemons
}
