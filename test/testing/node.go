// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testing

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/abci"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/testdata"
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
	SetModule("restore", "error").
	// SetModule("accumulate", "debug").
	// SetModule("executor", "debug").
	// SetModule("synthetic", "debug").
	// SetModule("anchoring", "info").
	// SetModule("block", "debug").
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
	cfg.Instrumentation.Prometheus = false               // Disable prometheus: https://github.com/tendermint/tendermint/issues/7076
	cfg.Accumulate.Network.Partitions = []config.Partition{
		{
			Id:   "local",
			Type: config.BlockValidator,
		},
	}

	cfg.LogLevel = DefaultLogLevels
	return cfg
}

func CreateTestNet(t testing.TB, numBvns, numValidators, numFollowers int, withFactomAddress bool) ([]string, map[string][]*accumulated.Daemon) {
	tempDir := t.TempDir()

	netInit := accumulated.NewDevnet(accumulated.DevnetOptions{
		BvnCount:       numBvns,
		ValidatorCount: numValidators,
		FollowerCount:  numFollowers,
		BasePort:       30000,
		GenerateKeys: func() (privVal, dnn, bvnn []byte) {
			return ed25519.GenPrivKey(), ed25519.GenPrivKey(), ed25519.GenPrivKey()
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

	// Disable the sliding fee schedule
	values := new(core.GlobalValues)
	values.Globals = new(protocol.NetworkGlobals)
	values.Globals.FeeSchedule = new(protocol.FeeSchedule)
	values.ExecutorVersion = protocol.ExecutorVersionV1SignatureAnchoring

	var factomAddresses func() (io.Reader, error)
	if withFactomAddress {
		factomAddresses = func() (io.Reader, error) { return strings.NewReader(testdata.FactomAddresses), nil }
	}
	genDocs, err := accumulated.BuildGenesisDocs(netInit, values, time.Now(), initLogger, factomAddresses, nil)
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

			err = accumulated.WriteNodeFiles(configs[i][j][0], node.PrivValKey, node.DnNodeKey, dnGenDoc)
			require.NoError(t, err)
			err = accumulated.WriteNodeFiles(configs[i][j][1], node.PrivValKey, node.BvnNodeKey, bvnGenDoc)
			require.NoError(t, err)
		}
	}

	daemons := make(map[string][]*accumulated.Daemon, numBvns+1)
	for _, configs := range configs {
		for _, configs := range configs {
			for _, cfg := range configs {
				daemon, err := accumulated.Load(cfg.RootDir, func(c *config.Config) (io.Writer, error) { return logWriter(c.LogFormat) })
				require.NoError(t, err)
				partition := cfg.Accumulate.PartitionId
				daemon.Logger = daemon.Logger.With("test", t.Name(), "partition", partition, "node", cfg.Moniker)
				daemons[partition] = append(daemons[partition], daemon)
			}
		}
	}

	partitionNames := []string{protocol.Directory}
	for _, bvn := range netInit.Bvns {
		partitionNames = append(partitionNames, bvn.Id)
	}

	return partitionNames, daemons
}

func RunTestNet(t testing.TB, partitions []string, daemons map[string][]*accumulated.Daemon) {
	t.Helper()
	for _, netName := range partitions {
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
		for _, netName := range partitions {
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

func BvnIdForTest(t testing.TB) string {
	id := t.Name()
	id = strings.ReplaceAll(id, "/", "-")
	id = strings.ReplaceAll(id, "#", "-")
	return id
}

func BvnUrlForTest(t testing.TB) *url.URL {
	return protocol.PartitionUrl(BvnIdForTest(t))
}
