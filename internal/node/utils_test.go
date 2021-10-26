package node_test

import (
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"reflect"
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

func initNodes(t *testing.T, baseIP net.IP, basePort int, count int, logLevel string) []*node.Node {
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
		config[i].Accumulate.Networks = []string{fmt.Sprintf("%s:%d", IPs[0], basePort+node.TmRpcPortOffset)}
		config[i].Consensus.CreateEmptyBlocks = false
	}

	workDir := t.TempDir()
	require.NoError(t, node.Init(node.InitOptions{
		WorkDir:   workDir,
		ShardName: t.Name(),
		ChainID:   t.Name(),
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

		nodes[i], _, err = acctesting.NewBVCNode(nodeDir, false, func(s string) zerolog.Logger {
			zl := logging.NewTestZeroLogger(t, s)
			zl = zl.With().Int("node", i).Logger()
			zl = zl.Hook(zerolog.HookFunc(zerologEventFilter))
			return zl
		}, t.Cleanup)
		require.NoError(t, err)
	}

	return nodes
}

func startNodes(t *testing.T, nodes []*node.Node) *api.Query {
	t.Helper()

	for _, n := range nodes {
		require.NoError(t, n.Start())
	}

	rpc, err := rpc.New(nodes[0].Config.RPC.ListenAddress)
	require.NoError(t, err)

	relay := relay.New(rpc)
	require.NoError(t, relay.Start())
	t.Cleanup(func() { require.NoError(t, relay.Stop()) })

	return api.NewQuery(relay)
}

func zerologEventFilter(e *zerolog.Event, level zerolog.Level, message string) {
	// if level > zerolog.InfoLevel {
	// 	return
	// }

	switch message {
	case "starting service", "stopping service":
		e.Discard()
		return
	}

	// This is the hackiest of hacks, but I want the buffer
	rv := reflect.ValueOf(e)
	buf := rv.Elem().FieldByName("buf").Bytes()
	buf = append(buf, '}')

	var v map[string]interface{}
	err := json.Unmarshal(buf, &v)
	if err != nil {
		return
	}

	module, ok := v["module"].(string)
	if !ok {
		return
	}

	switch module {
	case "rpc-server", "p2p", "rpc", "statesync":
		e.Discard()
	default:
		// OK
	}
}
