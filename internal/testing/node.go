package testing

import (
	"io"
	"net"
	"time"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/accumulated"
	"github.com/AccumulateNetwork/accumulate/internal/node"
)

func DefaultConfig(net config.NetworkType, node config.NodeType, netId string) *config.Config {
	cfg := config.Default(net, node, netId)        //
	cfg.Mempool.MaxBatchBytes = 1048576            //
	cfg.Mempool.CacheSize = 1048576                //
	cfg.Mempool.Size = 50000                       //
	cfg.Consensus.CreateEmptyBlocks = false        // Empty blocks are annoying to debug
	cfg.Consensus.TimeoutCommit = time.Second / 10 // Increase block frequency
	cfg.Accumulate.Website.Enabled = false         // No need for the website
	cfg.Instrumentation.Prometheus = false         // Disable prometheus: https://github.com/tendermint/tendermint/issues/7076
	cfg.Accumulate.Network.BvnNames = []string{netId}
	cfg.Accumulate.Network.Addresses = map[string][]string{netId: {"local"}}
	return cfg
}

func NodeInitOptsForLocalNetwork(name string, ip net.IP) node.InitOptions {
	// TODO Support DN, multi-BVN, multi-validator
	return node.InitOptions{
		Port:     30000,
		Config:   []*config.Config{DefaultConfig(config.BlockValidator, config.Validator, name)},
		RemoteIP: []string{ip.String()},
		ListenIP: []string{ip.String()},
	}
}

type DaemonOptions struct {
	Dir       string
	MemDB     bool
	LogWriter func(string) (io.Writer, error)
}

func RunDaemon(opts DaemonOptions, cleanup func(func())) (*accumulated.Daemon, error) {
	// Load the daemon
	daemon, err := accumulated.Load(opts.Dir, opts.LogWriter)
	if err != nil {
		return nil, err
	}

	// Set test knobs
	daemon.IsTest = true
	daemon.UseMemDB = opts.MemDB

	// Start the daemon
	err = daemon.Start()
	if err != nil {
		return nil, err
	}

	cleanup(func() {
		_ = daemon.Stop()
	})

	return daemon, nil
}
