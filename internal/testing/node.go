package testing

import (
	"fmt"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	cfg "gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/abci"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"gitlab.com/accumulatenetwork/accumulate/internal/genesis"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/sync/errgroup"
)

const LogConsole = true

func NewTestLogger(t testing.TB) log.Logger {
	if !LogConsole {
		return logging.NewTestLogger(t, "plain", DefaultLogLevels, false)
	}

	w, err := logging.NewConsoleWriter("plain")
	require.NoError(t, err)
	level, writer, err := logging.ParseLogLevel(DefaultLogLevels, w)
	require.NoError(t, err)
	logger, err := logging.NewTendermintLogger(zerolog.New(writer), level, false)
	require.NoError(t, err)
	return logger
}

var DefaultLogLevels = config.LogLevel{}.
	Parse(config.DefaultLogLevels).
	// SetModule("accumulate", "debug").
	SetModule("executor", "info").
	// SetModule("governor", "debug").
	// SetModule("synthetic", "debug").
	// SetModule("storage", "debug").
	// SetModule("database", "debug").
	// SetModule("fake-node", "debug").
	// SetModule("fake-tendermint", "info").
	String()

func DefaultConfig(networkName string, net config.NetworkType, node config.NodeType, netId string) *config.Config {
	cfg := config.Default(networkName, net, node, netId) //
	cfg.Mempool.MaxBatchBytes = 1048576                  //
	cfg.Mempool.CacheSize = 1048576                      //
	cfg.Mempool.Size = 50000                             //
	cfg.Consensus.CreateEmptyBlocks = false              // Empty blocks are annoying to debug
	cfg.Consensus.TimeoutCommit = time.Second / 5        // Increase block frequency
	cfg.Accumulate.Website.Enabled = false               // No need for the website
	cfg.Instrumentation.Prometheus = false               // Disable prometheus: https://github.com/tendermint/tendermint/issues/7076
	cfg.Accumulate.Network.Subnets = []config.Subnet{
		{
			Id:   "local",
			Type: config.BlockValidator,
		},
	}

	cfg.LogLevel = DefaultLogLevels
	return cfg
}

func CreateTestNet(t testing.TB, numBvns, numValidators, numFollowers int, withFactomAddress bool) ([]string, map[string][]*accumulated.Daemon) {
	const basePort = 30000
	tempDir := t.TempDir()

	count := numValidators + numFollowers
	subnets := make([]config.Subnet, numBvns+1)
	subnetsMap := make(map[string]config.Subnet)
	allAddresses := make(map[string][]string, numBvns+1)
	allConfigs := make(map[string][]*cfg.Config, numBvns+1)
	allRemotes := make(map[string][]string, numBvns+1)

	// Create node configurations
	for i := 0; i < numBvns+1; i++ {
		netType, subnetId := cfg.Directory, protocol.Directory
		if i > 0 {
			netType, subnetId = cfg.BlockValidator, fmt.Sprintf("BVN%d", i-1)
		}

		addresses := make([]string, count)
		configs := make([]*cfg.Config, count)
		remotes := make([]string, count)
		allAddresses[subnetId], allConfigs[subnetId], allRemotes[subnetId] = addresses, configs, remotes
		nodes := make([]config.Node, count)
		for i := 0; i < count; i++ {
			nodeType := cfg.Validator
			if i > numValidators {
				nodeType = cfg.Follower
			}

			hash := hashCaller(1, fmt.Sprintf("%s-%s-%d", t.Name(), subnetId, i))
			configs[i] = DefaultConfig("unittest", netType, nodeType, subnetId)
			remotes[i] = getIP(hash).String()
			nodes[i] = config.Node{
				Type:    nodeType,
				Address: remotes[i], //fmt.Sprintf("http://%s:%d", remotes[i], basePort),
			}
			addresses[i] = nodes[i].Address
		}

		// We need to return the subnets in a specific order with directory node first because the unit tests select subnets[1]
		subnets[i] = config.Subnet{
			Id:       subnetId,
			Type:     netType,
			BasePort: basePort,
			Nodes:    nodes,
		}
		subnetsMap[subnetId] = subnets[i]
	}

	// Add addresses and BVN names to node configurations
	for _, configs := range allConfigs {
		for i, cfg := range configs {
			cfg.Accumulate.Network.Subnets = subnets
			cfg.Accumulate.LocalAddress = allAddresses[cfg.Accumulate.SubnetId][i]
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
	var factomAddressFilePath string
	if withFactomAddress {
		factomAddressFilePath = "test_factom_addresses"
	}

	allDaemons := make(map[string][]*accumulated.Daemon, numBvns+1)
	netValMap := make(genesis.NetworkValidatorMap)
	var bootstrapList []genesis.Bootstrap

	for _, subnet := range subnets {
		subnetId := subnet.Id
		dir := filepath.Join(tempDir, subnetId)
		bootstrap, err := node.Init(node.InitOptions{
			WorkDir:             dir,
			Port:                basePort,
			Config:              allConfigs[subnetId],
			RemoteIP:            allRemotes[subnetId],
			ListenIP:            allRemotes[subnetId],
			NetworkValidatorMap: netValMap,
			Logger:              initLogger.With("subnet", subnetId),
			FactomAddressesFile: factomAddressFilePath,
		})
		require.NoError(t, err)
		if bootstrap != nil {
			bootstrapList = append(bootstrapList, bootstrap)
		}

		daemons := make([]*accumulated.Daemon, count)
		allDaemons[subnetId] = daemons

		for i := 0; i < count; i++ {
			dir := filepath.Join(dir, fmt.Sprintf("Node%d", i))
			var err error
			daemons[i], err = accumulated.Load(dir, logWriter)
			require.NoError(t, err)
			daemons[i].Logger = daemons[i].Logger.With("test", t.Name(), "subnet", subnetId, "node", i)
		}
	}

	// Execute bootstrap after the entire network is known
	for _, bootstrap := range bootstrapList {
		err := bootstrap.Bootstrap()
		if err != nil {
			panic(fmt.Errorf("could not execute genesis: %v", err))
		}
	}

	return getSubnetNames(subnets), allDaemons
}

func getSubnetNames(subnets []cfg.Subnet) []string {
	var res []string
	for _, subnet := range subnets {
		res = append(res, subnet.Id)
	}
	return res
}

func RunTestNet(t testing.TB, subnets []string, daemons map[string][]*accumulated.Daemon) {
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
				daemon := daemon // See docs/developer/rangevarref.md
				errg.Go(func() error {
					return daemon.Stop()
				})
			}
		}
		assert.NoError(t, errg.Wait())
	})
}
