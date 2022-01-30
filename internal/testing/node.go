package testing

import (
	"fmt"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulate/config"
	cfg "github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/abci"
	"github.com/AccumulateNetwork/accumulate/internal/accumulated"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/node"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	"golang.org/x/sync/errgroup"
)

const LogConsole = false

const DefaultLogLevels = config.DefaultLogLevels //+ ";accumulate=debug"

func DefaultConfig(net config.NetworkType, node config.NodeType, netId string) *config.Config {
	cfg := config.Default(net, node, netId)       //
	cfg.Mempool.MaxBatchBytes = 1048576           //
	cfg.Mempool.CacheSize = 1048576               //
	cfg.Mempool.Size = 50000                      //
	cfg.Consensus.CreateEmptyBlocks = false       // Empty blocks are annoying to debug
	cfg.Consensus.TimeoutCommit = time.Second / 5 // Increase block frequency
	cfg.Accumulate.Website.Enabled = false        // No need for the website
	cfg.Instrumentation.Prometheus = false        // Disable prometheus: https://github.com/tendermint/tendermint/issues/7076
	cfg.Accumulate.Network.BvnNames = []string{netId}
	cfg.Accumulate.Network.Addresses = map[string][]string{netId: {"local"}}
	cfg.LogLevel = DefaultLogLevels
	return cfg
}

func CreateTestNet(t *testing.T, numBvns, numValidators, numFollowers int) ([]string, map[string][]*accumulated.Daemon) {
	const basePort = 30000
	dir := t.TempDir()

	count := numValidators + numFollowers
	subnets := make([]string, numBvns+1)
	allAddresses := make(map[string][]string, numBvns+1)
	allConfigs := make(map[string][]*cfg.Config, numBvns+1)
	allRemotes := make(map[string][]string, numBvns+1)

	// Create node configurations
	for i := range subnets {
		netType, netName := cfg.Directory, protocol.Directory
		if i > 0 {
			netType, netName = cfg.BlockValidator, fmt.Sprintf("BVN%d", i)
		}
		subnets[i] = netName
		addresses := make([]string, count)
		configs := make([]*cfg.Config, count)
		remotes := make([]string, count)
		allAddresses[netName], allConfigs[netName], allRemotes[netName] = addresses, configs, remotes

		for i := 0; i < count; i++ {
			nodeType := cfg.Validator
			if i > numValidators {
				nodeType = cfg.Follower
			}

			hash := hashCaller(1, fmt.Sprintf("%s-%s-%d", t.Name(), netName, i))
			configs[i] = DefaultConfig(netType, nodeType, netName)
			remotes[i] = getIP(hash).String()
			addresses[i] = fmt.Sprintf("http://%s:%d", remotes[i], basePort)
		}

	}

	// Add addresses and BVN names to node configurations
	for _, configs := range allConfigs {
		for _, config := range configs {
			config.Accumulate.Network.BvnNames = subnets[1:]
			config.Accumulate.Network.Addresses = allAddresses
		}
	}

	var initLogger log.Logger
	var logWriter func(format string) (io.Writer, error)
	if LogConsole {
		logWriter = logging.NewConsoleWriter
		w, err := logging.NewConsoleWriter("plain")
		require.NoError(t, err)
		level, writer, err := logging.ParseLogLevel(DefaultLogLevels, w)
		require.NoError(t, err)
		initLogger, err = logging.NewTendermintLogger(zerolog.New(writer), level, false)
		require.NoError(t, err)
	} else {
		logWriter = logging.TestLogWriter(t)
		initLogger = logging.NewTestLogger(t, "plain", DefaultLogLevels, false)
	}

	allDaemons := make(map[string][]*accumulated.Daemon, numBvns+1)
	for _, netName := range subnets {
		dir := filepath.Join(dir, netName)
		require.NoError(t, node.Init(node.InitOptions{
			WorkDir:  dir,
			Port:     basePort,
			Config:   allConfigs[netName],
			RemoteIP: allRemotes[netName],
			ListenIP: allRemotes[netName],
			Logger:   initLogger.With("subnet", netName),
		}))

		daemons := make([]*accumulated.Daemon, count)
		allDaemons[netName] = daemons

		for i := 0; i < count; i++ {
			dir := filepath.Join(dir, fmt.Sprintf("Node%d", i))
			var err error
			daemons[i], err = accumulated.Load(dir, logWriter)
			require.NoError(t, err)
			daemons[i].Logger = daemons[i].Logger.With("test", t.Name(), "subnet", netName, "node", i)
		}
	}

	return subnets, allDaemons
}

func RunTestNet(t *testing.T, subnets []string, daemons map[string][]*accumulated.Daemon) {
	t.Helper()
	for _, netName := range subnets {
		for _, daemon := range daemons[netName] {
			require.NoError(t, daemon.Start())
			daemon.Node_TESTONLY().ABCI.(*abci.Accumulator).OnFatal(func(err error) {
				t.Helper()
				require.NoError(t, err)
			})
		}
	}
	t.Cleanup(func() {
		errg := new(errgroup.Group)
		for _, netName := range subnets {
			for _, daemon := range daemons[netName] {
				daemon := daemon
				errg.Go(func() error {
					return daemon.Stop()
				})
			}
		}
		assert.NoError(t, errg.Wait())
	})
}
