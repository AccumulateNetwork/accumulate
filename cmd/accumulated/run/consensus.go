// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	dbm "github.com/cometbft/cometbft-db"
	types "github.com/cometbft/cometbft/abci/types"
	tmcfg "github.com/cometbft/cometbft/config"
	tmcrypto "github.com/cometbft/cometbft/crypto"
	tmed25519 "github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cometbft/cometbft/crypto/tmhash"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/libs/log"
	tmnode "github.com/cometbft/cometbft/node"
	tmp2p "github.com/cometbft/cometbft/p2p"
	tmpv "github.com/cometbft/cometbft/privval"
	"github.com/cometbft/cometbft/proxy"
	"github.com/cometbft/cometbft/rpc/client"
	tmrpc "github.com/cometbft/cometbft/rpc/client"
	"github.com/cometbft/cometbft/rpc/client/local"
	tmtypes "github.com/cometbft/cometbft/types"
	"github.com/fatih/color"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multihash"
	"github.com/spf13/viper"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	tmlib "gitlab.com/accumulatenetwork/accumulate/exp/tendermint"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	nodecfg "gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	tmapi "gitlab.com/accumulatenetwork/accumulate/internal/api/v3/tm"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/crosschain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/abci"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var (
	consensusProvidesEventBus  = ioc.Provides[*events.Bus](func(c *ConsensusService) string { return c.App.partition().ID })
	consensusProvidesService   = ioc.Provides[v3.ConsensusService](func(c ConsensusApp) string { return c.partition().ID })
	consensusProvidesSubmitter = ioc.Provides[v3.Submitter](func(c ConsensusApp) string { return c.partition().ID })
	consensusProvidesValidator = ioc.Provides[v3.Validator](func(c ConsensusApp) string { return c.partition().ID })

	coreConsensusNeedsStorage      = ioc.Needs[keyvalue.Beginner](func(c *CoreConsensusApp) string { return c.Partition.ID })
	coreConsensusProvidesSequencer = ioc.Provides[private.Sequencer](func(c *CoreConsensusApp) string { return c.Partition.ID })
	coreConsensusProvidesRouter    = ioc.Provides[routing.Router](func(c *CoreConsensusApp) string { return c.Partition.ID })
	coreConsensusProvidesClient    = ioc.Provides[client.Client](func(c *CoreConsensusApp) string { return c.Partition.ID })
)

type tendermint struct {
	config       *tmcfg.Config
	privVal      *tmpv.FilePV
	nodeKey      *tmp2p.NodeKey
	logger       log.Logger
	eventBus     *events.Bus
	globals      chan *network.GlobalValues
	snapshotPath string // Path to genesis snapshot for ABCI InitChain
}

var _ prestarter = (*ConsensusService)(nil)

func (c *ConsensusService) Requires() []ioc.Requirement {
	return c.App.Requires()
}

func (c *ConsensusService) Provides() []ioc.Provided {
	return append(c.App.Provides(),
		consensusProvidesEventBus.Provided(c),
	)
}

func (c *ConsensusService) Verify() error {
	// Verify bootstrap peers can be converted to CometBFT format
	for i, peer := range c.BootstrapPeers {
		_, err := cmtPeerAddress(peer)
		if err != nil {
			return errors.UnknownError.WithFormat("bootstrap peer %d: %w", i, err)
		}
	}
	return nil
}

func (c *ConsensusService) prestart(inst *Instance) error {
	return c.App.prestart(inst)
}

func (c *ConsensusService) start(inst *Instance) error {
	// Note: MetricsNamespace is intentionally NOT set by default.
	// If MetricsNamespace is empty, Prometheus metrics will be disabled.
	// This prevents duplicate registration panics when running multiple tests.

	d := new(tendermint)
	d.logger = (*logging.Slogger)(inst.logger)
	d.eventBus = events.NewBus(d.logger.With("module", "events"))

	events.SubscribeAsync(d.eventBus, func(e events.FatalError) {
		slog.ErrorContext(inst.context, "Shutting down due to a fatal error", "error", e.Err)
		inst.shutdown()
	})

	// Create and register halt controller for this partition
	haltController := NewHaltController(
		c.App.partition().ID,
		inst.shutdown,
		inst.logger.With("module", "halt", "partition", c.App.partition().ID),
	)
	events.SubscribeSync(d.eventBus, haltController.OnDidCommitBlock)
	inst.RegisterHaltController(haltController)

	// Make the node directories
	err := os.MkdirAll(inst.path(c.NodeDir, "config"), 0700)
	if err != nil {
		return err
	}
	err = os.MkdirAll(inst.path(c.NodeDir, "data"), 0700)
	if err != nil {
		return err
	}

	// Load CometBFT config
	d.config = tmcfg.DefaultConfig()
	d.config.SetRoot(inst.path(c.NodeDir))
	// Disable Prometheus by default to prevent duplicate registration panics in tests
	d.config.Instrumentation.Prometheus = false
	_, err = os.Stat(inst.path(c.NodeDir, "config", "tendermint.toml"))
	switch {
	case err == nil:
		// Load the existing file with Viper because that's what Tendermint does
		nodeDir := inst.path(c.NodeDir)
		v := viper.New()
		v.SetConfigFile(filepath.Join(nodeDir, "config", "tendermint.toml"))
		v.AddConfigPath(filepath.Join(nodeDir, "config"))
		err = v.ReadInConfig()
		if err != nil {
			return err
		}

		err = v.Unmarshal(d.config)
		if err != nil {
			return err
		}

		// Only enable Prometheus if explicitly configured with a namespace
		if c.MetricsNamespace != "" && d.config.Instrumentation.Prometheus {
			d.config.Instrumentation.Namespace = c.MetricsNamespace
		} else {
			d.config.Instrumentation.Prometheus = false
		}

		// Process bootstrap peers from configuration if provided
		if len(c.BootstrapPeers) > 0 {
			inst.logger.Info("Updating persistent peers from bootstrap configuration",
				"count", len(c.BootstrapPeers))

			d.config.P2P.PersistentPeers = ""
			for i, peer := range c.BootstrapPeers {
				id, err := cmtPeerAddress(peer)
				if err != nil {
					return errors.UnknownError.WithFormat("bootstrap peer %d: %w", i, err)
				}
				if i > 0 {
					d.config.P2P.PersistentPeers += ","
				}
				d.config.P2P.PersistentPeers += id
			}

			inst.logger.Info("Persistent peers configured",
				"peers", d.config.P2P.PersistentPeers)
		}

		// P2P connection optimizations for stable block sync
		// Always apply these optimizations for better sync performance
		d.config.P2P.MaxNumOutboundPeers = 20
		if d.config.P2P.PersistentPeers != "" {
			// Extract node IDs from persistent peers (format: id@ip:port,id@ip:port,...)
			// UnconditionalPeerIDs only takes node IDs, not full addresses
			var ids []string
			for _, peer := range strings.Split(d.config.P2P.PersistentPeers, ",") {
				if idx := strings.Index(peer, "@"); idx > 0 {
					ids = append(ids, peer[:idx])
				}
			}
			d.config.P2P.UnconditionalPeerIDs = strings.Join(ids, ",")
		}
		d.config.P2P.SendRate = 20480000
		d.config.P2P.RecvRate = 20480000
		d.config.P2P.FlushThrottleTimeout = 50 * time.Millisecond
		d.config.P2P.PersistentPeersMaxDialPeriod = 30 * time.Second

		// Write updated config back to file
		tmcfg.WriteConfigFile(inst.path(c.NodeDir, "config", "tendermint.toml"), d.config)

	case errors.Is(err, fs.ErrNotExist):
		d.config.NodeKey = ""
		d.config.PrivValidatorKey = ""
		d.config.Genesis = filepath.Join("..", c.Genesis)
		d.config.Mempool.MaxTxBytes = 4194304

		// Only enable Prometheus metrics if a metrics namespace is explicitly configured
		// This prevents duplicate registration panics when running multiple tests
		if c.MetricsNamespace != "" {
			d.config.Instrumentation.Prometheus = true
			d.config.Instrumentation.PrometheusListenAddr = listenHostPort(c.Listen, defaultHost, portMetrics)
			d.config.Instrumentation.Namespace = c.MetricsNamespace
		}

		d.config.P2P.ListenAddress = listenUrl(c.Listen, defaultHost, useTCP{}, portCmtP2P)
		d.config.RPC.ListenAddress = listenUrl(c.Listen, defaultHost, useTCP{}, portCmtRPC)

		// No duplicate IPs
		d.config.P2P.AllowDuplicateIP = false

		// Initial peers (should be bootstrap peers but that setting isn't
		// present in 0.37)
		for i, peer := range c.BootstrapPeers {
			id, err := cmtPeerAddress(peer)
			if err != nil {
				return errors.UnknownError.WithFormat("bootstrap peer %d: %w", i, err)
			}
			if i > 0 {
				d.config.P2P.PersistentPeers += ","
			}
			d.config.P2P.PersistentPeers += id
		}

		// Set whether unroutable addresses are allowed
		d.config.P2P.AddrBookStrict = !isPrivate(c.Listen)

		// P2P connection optimizations for stable block sync
		// Increase outbound peers for better block sync source diversity
		d.config.P2P.MaxNumOutboundPeers = 20 // default 10

		// Unconditional peers - never disconnect from bootstrap peers
		// This prevents the disconnect/reconnect cycle during sync
		// Extract node IDs from persistent peers (format: id@ip:port,id@ip:port,...)
		if d.config.P2P.PersistentPeers != "" {
			var ids []string
			for _, peer := range strings.Split(d.config.P2P.PersistentPeers, ",") {
				if idx := strings.Index(peer, "@"); idx > 0 {
					ids = append(ids, peer[:idx])
				}
			}
			d.config.P2P.UnconditionalPeerIDs = strings.Join(ids, ",")
		}

		// Increase bandwidth limits for fast sync (default 5MB/s)
		d.config.P2P.SendRate = 20480000  // 20 MB/s
		d.config.P2P.RecvRate = 20480000  // 20 MB/s

		// Reduce flush throttle for more responsive connections
		d.config.P2P.FlushThrottleTimeout = 50 * time.Millisecond // default 100ms

		// Longer dial period for persistent peers to reduce reconnection churn
		d.config.P2P.PersistentPeersMaxDialPeriod = 30 * time.Second // default 0 (exponential)

		tmcfg.WriteConfigFile(inst.path(c.NodeDir, "config", "tendermint.toml"), d.config)

	default:
		return err
	}

	err = d.config.ValidateBasic()
	if err != nil {
		return err
	}

	// Load keys
	d.privVal, err = c.loadPrivVal(inst, d.config, c.ValidatorKey)
	if err != nil {
		return errors.UnknownError.WithFormat("load private validator key: %w", err)
	}

	d.nodeKey, err = convertNodeKey(inst)
	if err != nil {
		return errors.UnknownError.WithFormat("load node key: %w", err)
	}

	// Set the snapshot path for ABCI InitChain (only for snapshot files, not JSON)
	if filepath.Ext(c.Genesis) != ".json" {
		d.snapshotPath = inst.path(c.Genesis)
	}

	// Start the application
	app, err := c.App.start(inst, d)
	if err != nil {
		return err
	}

	// Start consensus
	node, err := tmnode.NewNode(
		d.config,
		d.privVal,
		d.nodeKey,
		proxy.NewLocalClientCreator(app),
		c.genesisDocProvider(inst),
		clearCachedGenesisDBProvider(d.logger),
		tmnode.DefaultMetricsProvider(d.config.Instrumentation),
		d.logger,
	)
	if err != nil {
		return errors.UnknownError.WithFormat("initialize consensus: %w", err)
	}

	err = node.Start()
	if err != nil {
		return errors.UnknownError.WithFormat("start consensus: %w", err)
	}

	inst.cleanup("consensus node", func(context.Context) error {
		err := node.Stop()
		node.Wait()
		return err
	})

	err = consensusProvidesEventBus.Register(inst.services, c, d.eventBus)
	if err != nil {
		return err
	}

	return c.App.register(inst, d, node)
}

func convertNodeKey(inst *Instance) (*tmp2p.NodeKey, error) {
	var key PrivateKey
	if inst.config.P2P != nil {
		key = inst.config.P2P.Key
		inst.logger.Info("P2P config found", "has_key", key != nil, "key_type", fmt.Sprintf("%T", key))

		// Log the key details for debugging
		if key != nil {
			switch k := key.(type) {
			case *RawPrivateKey:
				inst.logger.Info("P2P key is RawPrivateKey", "address", k.Address)
			case *CometNodeKeyFile:
				inst.logger.Info("P2P key is CometNodeKeyFile", "path", k.Path)
			case *TransientPrivateKey:
				inst.logger.Info("P2P key is TransientPrivateKey")
			case *PrivateKeySeed:
				inst.logger.Info("P2P key is PrivateKeySeed")
			default:
				inst.logger.Info("P2P key is unknown type")
			}
		}
	} else {
		inst.logger.Info("No P2P config - will fail")
	}

	key2, err := convertKeyToComet(inst, key, true) // Allow transient keys for P2P
	if err != nil {
		inst.logger.Error("Failed to convert key to CometBFT format", "error", err)
		return nil, err
	}

	nodeKey := &tmp2p.NodeKey{PrivKey: key2}
	nodeID := nodeKey.ID()
	inst.logger.Info("Node P2P identity generated",
		"node_id", string(nodeID),
		"node_id_short", string(nodeID)[:8]+"...",
		"pubkey", fmt.Sprintf("%X", nodeKey.PubKey().Bytes()))

	return nodeKey, nil
}

func (c *ConsensusService) loadPrivVal(inst *Instance, config *tmcfg.Config, key PrivateKey) (*tmpv.FilePV, error) {
	// Allow transient keys for followers (voting_power=0 since key not in genesis)
	allowTransient := false
	if _, ok := key.(*TransientPrivateKey); ok {
		inst.logger.Info("Using TransientPrivateKey for follower mode")
		allowTransient = true
	}

	key2, err := convertKeyToComet(inst, key, allowTransient)
	if err != nil {
		return nil, err
	}

	// This is a hack to work around CometBFT
	pv := tmpv.NewFilePV(key2, "", config.PrivValidatorStateFile())

	b, err := os.ReadFile(config.PrivValidatorStateFile())
	switch {
	case err == nil:
		err = cmtjson.Unmarshal(b, &pv.LastSignState)
		return pv, err
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}

	b, err = cmtjson.MarshalIndent(pv.LastSignState, "", "  ")
	if err != nil {
		return nil, err
	}
	err = os.WriteFile(config.PrivValidatorStateFile(), b, 0600)
	return pv, err
}

func convertKeyToComet(inst *Instance, key PrivateKey, allowTransient bool) (tmcrypto.PrivKey, error) {
	switch key.(type) {
	case nil:
		return nil, errors.BadRequest.With("key is nil")
	case *TransientPrivateKey:
		if !allowTransient {
			return nil, errors.BadRequest.With("key is transient")
		}
		// For P2P keys, transient keys are allowed (generates new identity each startup)
		inst.logger.Info("Allowing transient key for P2P (follower mode)")
	}

	addr, err := key.get(inst)
	if err != nil {
		return nil, err
	}

	sk, ok := addr.GetPrivateKey()
	if !ok {
		return nil, errors.BadRequest.With("not a private key")
	}

	switch addr.GetType() {
	case protocol.SignatureTypeED25519:
		return tmed25519.PrivKey(sk), nil
	default:
		return nil, errors.BadRequest.WithFormat("unsupported key type %v", addr.GetType())
	}
}

func (c *ConsensusService) genesisDocProvider(inst *Instance) tmnode.GenesisDocProvider {
	path := inst.path(c.Genesis)

	if filepath.Ext(c.Genesis) == ".json" {
		return func() (*tmtypes.GenesisDoc, error) {
			return tmtypes.GenesisDocFromFile(path)
		}
	}

	return func() (*tmtypes.GenesisDoc, error) {
		// Open the snapshot
		all, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		return genesis.ConvertSnapshotToJson(all)
	}
}

func cmtPeerAddress(addr multiaddr.Multiaddr) (string, error) {
	var pub *multihash.DecodedMultihash
	var host, port string
	var err error
	multiaddr.ForEach(addr, func(c multiaddr.Component) bool {
		switch c.Protocol().Code {
		case multiaddr.P_P2P:
			pub, err = multihash.Decode(c.RawValue())
		case multiaddr.P_IP4,
			multiaddr.P_IP6,
			multiaddr.P_DNS,
			multiaddr.P_DNS4,
			multiaddr.P_DNS6:
			host = c.Value()
		case multiaddr.P_TCP,
			multiaddr.P_UDP:
			port = c.Value()
		}
		if err != nil {
			return false
		}
		return pub == nil || host == "" || port == ""
	})
	if err != nil {
		return "", err
	}
	if pub == nil {
		return "", errors.BadRequest.With("missing peer ID")
	}
	if host == "" {
		return "", errors.BadRequest.With("missing host")
	}
	if port == "" {
		return "", errors.BadRequest.With("missing port")
	}

	// Convert libp2p port to CometBFT P2P port
	// libp2p uses port offset +2 (16593, 16693), CometBFT uses offset +0 (16591, 16691)
	portNum, err := strconv.Atoi(port)
	if err != nil {
		return "", errors.BadRequest.WithFormat("invalid port %q: %w", port, err)
	}
	cmtPort := portNum - 2
	if cmtPort < 1 || cmtPort > 65535 {
		return "", errors.BadRequest.WithFormat("adjusted port %d out of valid range", cmtPort)
	}

	var hash []byte
	switch pub.Code {
	case multihash.IDENTITY:
		p, err := crypto.UnmarshalPublicKey(pub.Digest)
		if err != nil {
			return "", errors.BadRequest.WithFormat("decode public key: %w", err)
		}
		b, err := p.Raw()
		if err != nil {
			return "", errors.BadRequest.WithFormat("unwrap public key: %w", err)
		}
		hash = tmhash.SumTruncated(b)
	case multihash.SHA2_256:
		hash = pub.Digest[:tmhash.TruncatedSize]
	default:
		return "", errors.BadRequest.WithFormat("unsupported multihash type %v", pub.Name)
	}
	return tmp2p.IDAddressString(tmp2p.ID(hex.EncodeToString(hash)), fmt.Sprintf("%s:%d", host, cmtPort)), nil
}

func (c *CoreConsensusApp) partition() *protocol.PartitionInfo { return c.Partition }

func (c *CoreConsensusApp) Requires() []ioc.Requirement {
	return []ioc.Requirement{
		coreConsensusNeedsStorage.Requirement(c),
	}
}

func (c *CoreConsensusApp) Provides() []ioc.Provided {
	return []ioc.Provided{
		consensusProvidesService.Provided(c),
		consensusProvidesSubmitter.Provided(c),
		consensusProvidesValidator.Provided(c),
		coreConsensusProvidesSequencer.Provided(c),
		coreConsensusProvidesRouter.Provided(c),
		coreConsensusProvidesClient.Provided(c),
	}
}

func (c *CoreConsensusApp) prestart(inst *Instance) error {
	return coreConsensusProvidesClient.Register(inst.services, c, tmlib.NewDeferredClient())
}

func (c *CoreConsensusApp) start(inst *Instance, d *tendermint) (types.Application, error) {
	setDefaultPtr(&c.EnableHealing, false)
	setDefaultPtr(&c.EnableDirectDispatch, true)
	setDefaultPtr(&c.MaxEnvelopesPerBlock, 100)

	store, err := coreConsensusNeedsStorage.Get(inst.services, c)
	if err != nil {
		return nil, err
	}

	router := routing.NewRouter(routing.RouterOptions{
		Events: d.eventBus,
		Logger: d.logger,
	})
	err = coreConsensusProvidesRouter.Register(inst.services, c, router)
	if err != nil {
		return nil, err
	}

	dialer := inst.p2p.DialNetwork()
	client := &message.Client{Transport: &message.RoutedTransport{
		Network: inst.config.Network,
		Dialer:  dialer,
		Router:  routing.MessageRouter{Router: router},
	}}
	db := database.New(store, d.logger)
	execOpts := execute.Options{
		Logger:        d.logger.With("module", "executor"),
		Database:      db,
		Key:           d.privVal.Key.PrivKey.Bytes(),
		Router:        router,
		EventBus:      d.eventBus,
		Sequencer:     client.Private(),
		Querier:       client,
		EnableHealing: *c.EnableHealing,
		Describe: execute.DescribeShim{
			NetworkType: c.Partition.Type,
			PartitionId: c.Partition.ID,
		},
	}

	// Why does this exist? Why not just use tmlib.DispatcherClient?
	type Client interface {
		tmrpc.ABCIClient
		tmrpc.NetworkClient
		tmrpc.MempoolClient
		tmrpc.StatusClient
	}

	clients := map[string]tmlib.DispatcherClient{}
	ioc.ForEach(inst.services, func(desc ioc.Descriptor, svc Client) {
		clients[strings.ToLower(desc.Namespace())] = svc
	})

	if _, ok := clients["directory"]; !ok ||
		!*c.EnableDirectDispatch {
		// If we are not attached to a DN node, or direct dispatch is disabled,
		// use the API dispatcher
		execOpts.NewDispatcher = func() execute.Dispatcher {
			return accumulated.NewDispatcher(inst.config.Network, router, dialer)
		}

	} else {
		// Otherwise, use the Tendermint dispatcher
		execOpts.NewDispatcher = func() execute.Dispatcher {
			return tmlib.NewDispatcher(router, clients)
		}
	}

	// Setup globals
	d.globals = make(chan *network.GlobalValues, 1)
	events.SubscribeSync(d.eventBus, func(e events.WillChangeGlobals) error {
		select {
		case d.globals <- e.New:
		default:
		}
		return nil
	})

	// This must happen before creating the executor since it needs to receive
	// the initial WillChangeGlobals event
	conductor := &crosschain.Conductor{
		Partition:    c.Partition,
		ValidatorKey: execOpts.Key,
		Database:     execOpts.Database,
		Querier:      v3.Querier2{Querier: client},
		Dispatcher:   execOpts.NewDispatcher(),
		RunTask:      execOpts.BackgroundTaskLauncher,

		// TODO Fix the flooding issues and enable this by default
		EnableAnchorHealing: Ptr(false),
	}
	err = conductor.Start(d.eventBus)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("start conductor: %v", err)
	}

	exec, err := execute.NewExecutor(execOpts)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("initialize chain executor: %w", err)
	}

	// If a SnapshotService is registered for this partition, surface
	// its directory to the ABCI snapshot hooks so this node can serve
	// snapshots via CometBFT state-sync. Without this, ABCI ListSnapshots
	// has no idea where to look and either nil-panics or returns empty.
	//
	// SnapshotService.Directory is workdir-relative (e.g. "dnn/snapshots"),
	// but the ABCI Accumulator's RootDir is the per-partition node dir
	// (e.g. <workdir>/dnn). Strip the leading nodeDir component so
	// MakeAbsolute(nodeDir, dir) doesn't produce <workdir>/dnn/dnn/snapshots.
	var snapshotsCfg *nodecfg.Snapshots
	for _, s := range inst.config.Services {
		ss, ok := s.(*SnapshotService)
		if !ok {
			continue
		}
		if !strings.EqualFold(ss.Partition, c.Partition.ID) {
			continue
		}
		dir := ss.Directory
		// Strip the leading nodeDir component if present (e.g.
		// "dnn/snapshots" → "snapshots") since ABCI Accumulator's
		// MakeAbsolute uses RootDir = <workdir>/<nodeDir> already.
		nodeDirName := filepath.Base(d.config.RootDir)
		if rel, err := filepath.Rel(nodeDirName, ss.Directory); err == nil && !strings.HasPrefix(rel, "..") {
			dir = rel
		}
		snapshotsCfg = &nodecfg.Snapshots{
			Enable:    true,
			Directory: dir,
		}
		break
	}

	app := abci.NewAccumulator(abci.AccumulatorOptions{
		ID:           inst.id,
		Address:      d.privVal.Key.PubKey.Address(),
		Executor:     exec,
		Logger:       d.logger.With("module", "abci"),
		EventBus:     d.eventBus,
		Database:     db,
		Genesis:      genesis.DocProvider(d.config),
		Partition:    c.Partition.ID,
		RootDir:      d.config.RootDir,
		Snapshots:    snapshotsCfg,
		SnapshotPath: d.snapshotPath,

		MaxEnvelopesPerBlock: int(*c.MaxEnvelopesPerBlock),
	})
	return app, nil
}

func (c *CoreConsensusApp) register(inst *Instance, d *tendermint, node *tmnode.Node) error {
	store, err := coreConsensusNeedsStorage.Get(inst.services, c)
	if err != nil {
		return err
	}

	// Clear the AppState from the genesis doc to free ~2GB of memory.
	// This is safe because InitChain (which needs AppState) has already run
	// during node.Start(), or will load the snapshot from disk (for new nodes).
	// Both Environment #1 (from startRPC) and Environment #2 (from local.New)
	// share the same GenesisDoc pointer, so this clears AppState for both.
	// Note: Environment #1's genChunks are already created with the full AppState
	// but we can't easily clear those (~7GB). This at least frees the raw AppState.
	if genDoc := node.GenesisDoc(); genDoc != nil && len(genDoc.AppState) > 0 {
		d.logger.Info("Clearing genesis AppState to free memory", "size", len(genDoc.AppState))
		genDoc.AppState = nil
	}

	// Register the tendermint node
	localClient := local.New(node)

	// Clear the genesis cache to free ~10GB of memory. This must be done
	// after local.New() which triggers ConfigureRPC() and InitGenesisChunks().
	clearGenesisCache(localClient, d.logger)

	// Start sync monitor to detect and recover from stuck sync (CometBFT bug workaround)
	go func() {
		monitor := accumulated.NewSyncMonitor(
			&consensusStatusProvider{localClient},
			node.Switch(),
			d.config.P2P.PersistentPeers,
		)

		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-inst.context.Done():
				return
			case <-t.C:
				_, err := monitor.Check(inst.context)
				if err != nil {
					slog.ErrorContext(inst.context, "Sync monitor check failed", "error", err)
				}
			}
		}
	}()

	err = coreConsensusProvidesClient.Register(inst.services, c, localClient)
	if err != nil {
		return err
	}

	// Register the consensus service
	svcImpl := tmapi.NewConsensusService(tmapi.ConsensusServiceParams{
		Logger:           d.logger.With("module", "api"),
		Local:            localClient,
		Database:         database.New(store, d.logger),
		PartitionID:      c.Partition.ID,
		PartitionType:    c.Partition.Type,
		EventBus:         d.eventBus,
		NodeKeyHash:      sha256.Sum256(d.nodeKey.PubKey().Bytes()),
		ValidatorKeyHash: sha256.Sum256(d.privVal.Key.PubKey.Bytes()),
	})
	registerRpcService(inst, svcImpl.Type().AddressFor(c.Partition.ID), message.ConsensusService{ConsensusService: svcImpl})
	err = consensusProvidesService.Register(inst.services, c, svcImpl)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Register the submitter
	subImpl := tmapi.NewSubmitter(tmapi.SubmitterParams{
		Logger: d.logger.With("module", "api"),
		Local:  localClient,
	})
	registerRpcService(inst, subImpl.Type().AddressFor(c.Partition.ID), message.Submitter{Submitter: subImpl})
	err = consensusProvidesSubmitter.Register(inst.services, c, subImpl)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Register the validator
	valImpl := tmapi.NewValidator(tmapi.ValidatorParams{
		Logger: d.logger.With("module", "api"),
		Local:  localClient,
	})
	registerRpcService(inst, valImpl.Type().AddressFor(c.Partition.ID), message.Validator{Validator: valImpl})
	err = consensusProvidesValidator.Register(inst.services, c, valImpl)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Register the sequencer
	seqImpl := api.NewSequencer(api.SequencerParams{
		Logger:       d.logger.With("module", "api"),
		Database:     database.New(store, d.logger),
		EventBus:     d.eventBus,
		Globals:      <-d.globals,
		Partition:    c.Partition.ID,
		ValidatorKey: d.privVal.Key.PrivKey.Bytes(),
	})
	registerRpcService(inst, seqImpl.Type().AddressFor(c.Partition.ID), message.Sequencer{Sequencer: seqImpl})
	err = coreConsensusProvidesSequencer.Register(inst.services, c, seqImpl)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	inst.logger.Info(color.HiBlueString("Running"), "partition", c.Partition.ID, "module", "run", "service", "consensus")
	return nil
}

// clearCachedGenesisDBProvider wraps CometBFT's DefaultDBProvider to delete
// the cached genesis document from the state database. CometBFT caches the full
// genesis (including the 2.68GB AppState) to the database on first run and
// loads it on every subsequent start. By deleting this cached copy, we force
// CometBFT to call our genesisDocProvider which returns null AppState.
func clearCachedGenesisDBProvider(logger log.Logger) tmcfg.DBProvider {
	return func(ctx *tmcfg.DBContext) (dbm.DB, error) {
		logger.Info("DBProvider called", "id", ctx.ID)
		db, err := tmcfg.DefaultDBProvider(ctx)
		if err != nil {
			return nil, err
		}

		// Only clear from the state database
		if ctx.ID == "state" {
			genesisDocKey := []byte("genesisDoc")
			has, err := db.Has(genesisDocKey)
			if err != nil {
				logger.Error("Failed to check for cached genesis", "error", err)
			} else if has {
				logger.Info("Deleting cached genesis from state database to free memory")
				if err := db.Delete(genesisDocKey); err != nil {
					logger.Error("Failed to delete cached genesis", "error", err)
				} else {
					logger.Info("Successfully deleted cached genesis from state database")
				}
			} else {
				logger.Info("No cached genesis found in state database")
			}
		}

		return db, nil
	}
}

// clearGenesisCache clears the cached genesis data from CometBFT's RPC
// environment to free ~10GB of memory. The genesis data is only needed during
// initialization and for the rarely-used genesis RPC endpoints.
func clearGenesisCache(localClient *local.Local, logger log.Logger) {
	// Use reflection with unsafe to access and clear private fields in Local.env
	localVal := reflect.ValueOf(localClient).Elem()
	envField := localVal.FieldByName("env")
	if !envField.IsValid() {
		logger.Error("Failed to clear genesis cache: env field not found")
		return
	}

	// Get the Environment pointer using unsafe to bypass unexported restrictions
	envPtrVal := reflect.NewAt(envField.Type(), envField.Addr().UnsafePointer()).Elem()
	if envPtrVal.IsNil() {
		return
	}

	env := envPtrVal.Elem()

	// Clear genChunks ([]string) - use unsafe to set unexported field
	genChunksField := env.FieldByName("genChunks")
	if genChunksField.IsValid() {
		// Create a settable version using unsafe
		genChunksPtr := reflect.NewAt(genChunksField.Type(), genChunksField.Addr().UnsafePointer()).Elem()
		genChunksPtr.Set(reflect.Zero(genChunksField.Type()))
		logger.Info("Cleared genesis chunks cache")
	}

	// Clear GenDoc (*types.GenesisDoc) - use unsafe to set unexported field
	genDocField := env.FieldByName("GenDoc")
	if genDocField.IsValid() {
		genDocPtr := reflect.NewAt(genDocField.Type(), genDocField.Addr().UnsafePointer()).Elem()
		genDocPtr.Set(reflect.Zero(genDocField.Type()))
		logger.Info("Cleared genesis doc cache")
	}
}

// consensusStatusProvider adapts the CometBFT local client to the StatusProvider interface
type consensusStatusProvider struct {
	client *local.Local
}

func (p *consensusStatusProvider) Status(ctx context.Context) (*accumulated.SyncStatus, error) {
	st, err := p.client.Status(ctx)
	if err != nil {
		return nil, err
	}
	return &accumulated.SyncStatus{
		CatchingUp:        st.SyncInfo.CatchingUp,
		LatestBlockHeight: st.SyncInfo.LatestBlockHeight,
		LatestBlockTime:   st.SyncInfo.LatestBlockTime,
	}, nil
}
