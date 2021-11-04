package node_test

import (
	"fmt"
	"net"
	"path/filepath"
	"testing"

	"github.com/AccumulateNetwork/accumulated/config"
	cfg "github.com/AccumulateNetwork/accumulated/config"
	"github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/internal/logging"
	"github.com/AccumulateNetwork/accumulated/internal/node"
	"github.com/AccumulateNetwork/accumulated/internal/relay"
	acctesting "github.com/AccumulateNetwork/accumulated/internal/testing"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	rpc "github.com/tendermint/tendermint/rpc/client/http"
)

func initNodes(t *testing.T, workDir, name string, baseIP net.IP, basePort int, count int, logLevel string, relay []string) []*node.Node {
	t.Helper()

	IPs := make([]string, count)
	config := make([]*config.Config, count)

	for i := range IPs {
		ip := make(net.IP, len(baseIP))
		copy(ip, baseIP)
		ip[15] += byte(i)
		IPs[i] = fmt.Sprintf("tcp://%v", ip)
	}

	for i := range config {
		config[i] = cfg.DefaultValidator()
		if relay != nil {
			config[i].Accumulate.Networks = make([]string, len(relay))
			for j, r := range relay {
				config[i].Accumulate.Networks[j] = fmt.Sprintf("tcp://%s:%d", r, basePort+node.TmRpcPortOffset)
			}
		} else {
			config[i].Accumulate.Networks = []string{fmt.Sprintf("%s:%d", IPs[0], basePort+node.TmRpcPortOffset)}
		}
		config[i].Consensus.CreateEmptyBlocks = false
		config[i].Accumulate.API.EnableSubscribeTX = true
	}

	require.NoError(t, node.Init(node.InitOptions{
		WorkDir:   workDir,
		ShardName: name,
		ChainID:   name,
		Port:      basePort,
		Config:    config,
		RemoteIP:  IPs,
		ListenIP:  IPs,
	}))

	nodes := make([]*node.Node, count)
	for i := range nodes {
		nodeDir := filepath.Join(workDir, fmt.Sprintf("Node%d", i))
		c, err := cfg.Load(nodeDir)
		require.NoError(t, err)

		c.Instrumentation.Prometheus = false
		c.Accumulate.WebsiteEnabled = false
		c.LogLevel = logLevel

		require.NoError(t, cfg.Store(c))
		nodes[i] = newNode(t, workDir, i, c)
	}

	return nodes
}

func newNode(t *testing.T, workDir string, nodeNum int, config *config.Config) *node.Node {
	nodeDir := filepath.Join(workDir, fmt.Sprintf("Node%d", nodeNum))
	n, _, _, err := acctesting.NewBVCNode(nodeDir, false, config.Accumulate.Networks, func(s string) zerolog.Logger {
		zl := logging.NewTestZeroLogger(t, s)
		zl = zl.With().Int("node", nodeNum).Logger()
		zl = zl.Hook(logging.ExcludeMessages("starting service", "stopping service"))
		zl = zl.Hook(logging.BodyHook(func(e *zerolog.Event, _ zerolog.Level, body map[string]interface{}) {
			module, ok := body["module"].(string)
			if !ok {
				return
			}

			switch module {
			case "rpc-server", "p2p", "rpc", "statesync":
				e.Discard()
			default:
				e.Discard()
			case "accumulate":
				// OK
			}
		}))
		return zl
	}, t.Cleanup)
	require.NoError(t, err)
	return n
}

func startNodes(t *testing.T, nodes []*node.Node) *api.Query {
	t.Helper()

	for _, n := range nodes {
		require.NoError(t, n.Start())
		t.Cleanup(func() {
			n.Stop()
			n.Wait()
		})
	}

	rpc, err := rpc.New(nodes[0].Config.RPC.ListenAddress)
	require.NoError(t, err)

	relay := relay.New(rpc)
	if nodes[0].Config.Accumulate.API.EnableSubscribeTX {
		require.NoError(t, relay.Start())
		t.Cleanup(func() { require.NoError(t, relay.Stop()) })
	}

	return api.NewQuery(relay)
}
