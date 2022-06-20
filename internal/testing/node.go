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
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	cfg "gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/abci"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
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

func CreateTestNet(t *testing.T, numBvns, numValidators, numFollowers int, withFactomAddress bool) ([]string, map[string][]*accumulated.Daemon) {
	tempDir := t.TempDir()

	netInit := accumulated.NewDevnet(accumulated.DevnetOptions{
		BvnCount:       numBvns,
		ValidatorCount: numValidators,
		FollowerCount:  numFollowers,
		BasePort:       30000,
		GenerateKeys: func() (privVal []byte, node []byte) {
			return ed25519.GenPrivKey(), ed25519.GenPrivKey()
		},
		HostName: func(bvnNum, nodeNum int) (host string, listen string) {
			hash := hashCaller(1, fmt.Sprintf("%s-%s-%d", t.Name(), fmt.Sprintf("BVN%d", bvnNum+1), nodeNum))
			return getIP(hash).String(), ""
		},
	})

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
	genDocs, err := accumulated.BuildGenesisDocs(netInit, new(core.GlobalValues), time.Now(), initLogger, factomAddressFilePath)
	require.NoError(t, err)

	configs := accumulated.BuildNodesConfig(netInit, DefaultConfig)
	var count int
	dnGenDoc := genDocs[protocol.Directory]
	for i, bvn := range netInit.Bvns {
		bvnGenDoc := genDocs[bvn.Id]
		for j, node := range bvn.Nodes {
			count++
			configs[i][j][0].SetRoot(filepath.Join(tempDir, fmt.Sprintf("node-%d", count), "dnn"))
			configs[i][j][1].SetRoot(filepath.Join(tempDir, fmt.Sprintf("node-%d", count), "bvnn"))

			err = accumulated.WriteNodeFiles(configs[i][j][0], node.PrivValKey, node.NodeKey, dnGenDoc)
			require.NoError(t, err)
			err = accumulated.WriteNodeFiles(configs[i][j][1], node.PrivValKey, node.NodeKey, bvnGenDoc)
			require.NoError(t, err)
		}
	}

	daemons := make(map[string][]*accumulated.Daemon, numBvns+1)
	for _, configs := range configs {
		for _, configs := range configs {
			for _, config := range configs {
				daemon, err := accumulated.Load(config.RootDir, func(c *cfg.Config) (io.Writer, error) { return logWriter(c.LogFormat) })
				require.NoError(t, err)
				subnet := config.Accumulate.SubnetId
				daemon.Logger = daemon.Logger.With("test", t.Name(), "subnet", subnet, "node", config.Moniker)
				daemons[subnet] = append(daemons[subnet], daemon)
			}
		}
	}

	subnetNames := []string{protocol.Directory}
	for _, bvn := range netInit.Bvns {
		subnetNames = append(subnetNames, bvn.Id)
	}

	return subnetNames, daemons
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
				daemon := daemon // See docs/developer/rangevarref.md
				errg.Go(func() error {
					return daemon.Stop()
				})
			}
		}
		assert.NoError(t, errg.Wait())
	})
}
