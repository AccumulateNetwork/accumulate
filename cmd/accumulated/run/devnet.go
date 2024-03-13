// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"crypto/ed25519"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tmtypes "github.com/cometbft/cometbft/types"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/exp/faucet"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const devNetDefaultHost = "/ip4/127.0.1.1"

var devnetAsset = regexp.MustCompile(`^` +
	`(` + `bootstrap` + //                Bootstrap node
	`|` + `bvn\d+-\d+` + //               Core node
	`|` + `(dn|bvn\d+)-genesis.snap` + // Genesis snapshot
	`)$`)

var _ resetable = (*DevnetConfiguration)(nil)

func (d *DevnetConfiguration) reset(inst *Instance) error {
	entries, err := os.ReadDir(inst.rootDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !devnetAsset.MatchString(entry.Name()) {
			continue
		}
		err = os.RemoveAll(inst.path(entry.Name()))
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *DevnetConfiguration) apply(inst *Instance, cfg *Config) error {
	// Validate
	if cfg.Network == "" {
		return errors.BadRequest.With("must specify the network")
	}
	if d.Listen != nil && !addrHasOneOf(d.Listen, "tcp", "udp") {
		return errors.BadRequest.With("listen address must specify a port")
	}
	if cfg.P2P == nil || cfg.P2P.Key == nil {
		return errors.BadRequest.With("must specify a P2P key")
	}
	switch cfg.P2P.Key.(type) {
	case *TransientPrivateKey:
		return errors.BadRequest.With("cannot use a transient P2P key")
	}

	// Apply defaults
	setDefaultVal(&d.Bvns, 2)
	setDefaultVal(&d.Validators, 2)
	setDefaultVal(&d.Listen, mustParseMulti("/tcp/26656"))

	// Prepare nodes
	perPart := int(d.Validators) + int(d.Followers)
	nodes := make([][]*nodeOpts, d.Bvns)
	for bvn := range nodes {
		nodes[bvn] = make([]*nodeOpts, perPart)
		for node := range nodes[bvn] {
			// Generate keys
			nodeKey, err := d.generateKey(inst, cfg, bvn, node, "node")
			if err != nil {
				return err
			}
			privVal, err := d.generateKey(inst, cfg, bvn, node, "privVal")
			if err != nil {
				return err
			}

			libp2pNodeKey, err := libp2pcrypto.UnmarshalEd25519PublicKey(nodeKey[32:])
			if err != nil {
				panic(err)
			}
			peerID, err := peer.IDFromPublicKey(libp2pNodeKey)
			if err != nil {
				panic(err)
			}

			nodes[bvn][node] = &nodeOpts{
				DevNet:  d,
				BVN:     bvn + 1,
				Node:    node + 1,
				IsVal:   node < int(d.Validators),
				IP:      1 + ipOffset(bvn*perPart+node),
				PeerID:  peerID,
				NodeKey: nodeKey,
				PrivVal: privVal,
			}
		}
	}

	// List all peer addresses for each partition
	var dnPeers []multiaddr.Multiaddr
	bvnPeers := make([][]multiaddr.Multiaddr, d.Bvns)
	for bvn := range nodes {
		for node := range nodes[bvn] {
			n := nodes[bvn][node]
			dnPeers = append(dnPeers, addrForPeer(listen(d.Listen, devNetDefaultHost, n.IP, useTCP{}, portDir, portCmtP2P), n.PeerID))
			bvnPeers[bvn] = append(bvnPeers[bvn], addrForPeer(listen(d.Listen, devNetDefaultHost, n.IP, useTCP{}, portBVN, portCmtP2P), n.PeerID))
		}
	}

	// Set the bootstrap peers parameters
	for bvn := range nodes {
		for node := range nodes[bvn] {
			nodes[bvn][node].DnBootstrap = dnPeers
			nodes[bvn][node].BvnBootstrap = bvnPeers[bvn]
		}
	}

	// Generate genesis documents
	err := d.buildGenesis(inst, cfg, nodes)
	if err != nil {
		return err
	}

	// Construct nodes
	err = d.applyBootstrap(inst, cfg, 0)
	if err != nil {
		return err
	}

	for _, nodes := range nodes {
		for _, node := range nodes {
			err = node.apply(inst, cfg)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *DevnetConfiguration) buildGenesis(inst *Instance, cfg *Config, nodes [][]*nodeOpts) error {
	v := setDefaultVal(&d.Globals, new(network.GlobalValues))

	// Set the executor version
	setDefaultVal(&v.ExecutorVersion, protocol.ExecutorVersionLatest)

	// Set the oracle to 1000 credits/ACME
	setDefaultVal(&v.Oracle, new(protocol.AcmeOracle))
	setDefaultVal(&v.Oracle.Price, 1000*protocol.AcmeOraclePrecision)

	// Set default globals
	r2_3 := protocol.Rational{Numerator: 2, Denominator: 3}
	g := setDefaultVal(&v.Globals, new(protocol.NetworkGlobals))  // *General*
	setDefaultVal(&g.OperatorAcceptThreshold, r2_3)               //   Operator threshold        = 2/3
	setDefaultVal(&g.ValidatorAcceptThreshold, r2_3)              //   Validator threshold       = 2/3
	setDefaultVal(&g.MajorBlockSchedule, "0 */12 * * *")          //   Major blocks              = 0:00 and 12:00
	f := setDefaultVal(&g.FeeSchedule, new(protocol.FeeSchedule)) // *Fees*
	setDefaultSlice(&f.CreateIdentitySliding, 500000)             //   1-letter ADI              = $50
	setDefaultVal(&f.CreateSubIdentity, 10000)                    //   Sub-ADI                   = $1
	l := setDefaultVal(&g.Limits, new(protocol.NetworkLimits))    // *Limits*
	setDefaultVal(&l.DataEntryParts, 100)                         //   Parts in a data entry     = 100
	setDefaultVal(&l.AccountAuthorities, 20)                      //   Authorities of an account = 20
	setDefaultVal(&l.BookPages, 20)                               //   Pages in a book           = 20
	setDefaultVal(&l.PageEntries, 100)                            //   Entries in a page         = 100
	setDefaultVal(&l.IdentityAccounts, 1000)                      //   Accounts in an ADI        = 1000

	// Build the network definition
	n := setDefaultVal(&v.Network, new(protocol.NetworkDefinition))
	n.NetworkName = cfg.Network
	n.AddPartition(protocol.Directory, protocol.PartitionTypeDirectory)
	for bvn := 0; bvn < int(d.Bvns); bvn++ {
		n.AddPartition(fmt.Sprintf("BVN%d", bvn+1), protocol.PartitionTypeBlockValidator)
	}
	for bvn := range nodes {
		for node := range nodes[bvn] {
			m := nodes[bvn][node]
			n.AddValidator(m.PrivVal[32:], protocol.Directory, true)
			n.AddValidator(m.PrivVal[32:], fmt.Sprintf("BVN%d", bvn+1), true)
		}
	}

	// Generate the faucet account
	faucetKey, err := d.generateKey(inst, cfg, "faucet")
	if err != nil {
		return err
	}
	faucetUrl, err := protocol.LiteTokenAddress(faucetKey[32:], "ACME", protocol.SignatureTypeED25519)
	if err != nil {
		return err
	}
	faucetSnapshot, err := faucet.CreateLite(faucetUrl)
	if err != nil {
		return err
	}

	// Use the main key as the operator
	mainKey, err := cfg.P2P.Key.get(inst)
	if err != nil {
		return err
	}
	mainPubKey, ok := mainKey.GetPublicKey()
	if !ok {
		return errors.BadRequest.With("main key does not have a public key")
	}

	// Build genesis documents
	for _, part := range n.Partitions {
		var path string
		if part.Type == protocol.PartitionTypeDirectory {
			path = inst.path("dn-genesis.snap")
		} else {
			path = inst.path(strings.ToLower(part.ID) + "-genesis.snap")
		}
		st, err := os.Stat(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			// File does not exist, so create it
			break
		case err != nil:
			// Unknown error
			return err
		case st.IsDir():
			return fmt.Errorf("cannot create genesis document: %s is a directory", path)
		default:
			// File exists, do not recreate it
			continue
		}
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()

		err = genesis.Init(f, genesis.InitOpts{
			NetworkID:       cfg.Network,
			PartitionId:     part.ID,
			NetworkType:     part.Type,
			GenesisTime:     time.Now(),
			Logger:          (*logging.Slogger)(inst.logger).With("partition", part.ID),
			GenesisGlobals:  v,
			OperatorKeys:    [][]byte{mainPubKey},
			ConsensusParams: tmtypes.DefaultConsensusParams(),
			Snapshots: []func() (ioutil.SectionReader, error){
				func() (ioutil.SectionReader, error) { return ioutil.NewBuffer(faucetSnapshot), nil },
			},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *DevnetConfiguration) generateKey(inst *Instance, cfg *Config, v ...any) ([]byte, error) {
	mainKey, err := cfg.P2P.Key.get(inst)
	if err != nil {
		return nil, err
	}
	mainKeyHash, ok := mainKey.GetPublicKeyHash()
	if !ok {
		panic("invalid main node key")
	}

	seed := record.KeyFromHash(*(*[32]byte)(mainKeyHash)).Append(v...).Hash()
	sk := ed25519.NewKeyFromSeed(seed[:])
	return sk, nil
}

func rawPrivKeyFrom(sk []byte) PrivateKey {
	addr := address.FromED25519PrivateKey(sk)
	return &RawPrivateKey{Address: addr.String()}
}

func addrForPeer(addr multiaddr.Multiaddr, id peer.ID) multiaddr.Multiaddr {
	c, err := multiaddr.NewComponent("p2p", id.String())
	if err != nil {
		panic(err)
	}
	return addr.Encapsulate(c)
}

func (d *DevnetConfiguration) applyBootstrap(inst *Instance, root *Config, ip ipOffset) error {
	cfg, sub, err := d.addSubNode(inst, root, "bootstrap")
	if err != nil {
		return err
	}

	sk, err := d.generateKey(inst, root, "bootstrap")
	if err != nil {
		return err
	}
	cfg.P2P.Key = rawPrivKeyFrom(sk)

	// When running the bootstrap node solo, set the discovery mode to auto
	cfg.P2P.DiscoveryMode = Ptr(DhtMode(dht.ModeAutoServer))

	// Router
	addService(cfg, &RouterService{}, func(*RouterService) string { return "" })

	// HTTP API
	http := addService(cfg, &HttpService{}, func(*HttpService) string { return "" })
	setDefaultVal(&http.Router, ServiceReference[*RouterService](""))
	setDefaultVal(&http.Listen, []multiaddr.Multiaddr{
		listen(d.Listen, devNetDefaultHost, ip, portAccAPI, useHTTP{}),
	})

	// Faucet
	faucetKey, err := d.generateKey(inst, root, "faucet")
	if err != nil {
		return err
	}
	faucetUrl, err := protocol.LiteTokenAddress(faucetKey[32:], "ACME", protocol.SignatureTypeED25519)
	if err != nil {
		return err
	}
	faucet := addService(cfg, &FaucetService{
		Account: faucetUrl,
	}, func(f *FaucetService) string { return f.Account.String() })
	setDefaultVal(&faucet.SigningKey, rawPrivKeyFrom(faucetKey))
	setDefaultVal(&faucet.Router, ServiceReference[*RouterService](""))
	inst.logger.Info("Faucet", "account", faucetUrl)

	return d.writeSubNode(inst, root, cfg, sub, ip)
}

type nodeOpts struct {
	DevNet       *DevnetConfiguration
	BVN          int
	Node         int
	IsVal        bool
	IP           ipOffset
	PeerID       peer.ID
	NodeKey      []byte
	PrivVal      []byte
	DnBootstrap  []multiaddr.Multiaddr
	BvnBootstrap []multiaddr.Multiaddr
}

func (n nodeOpts) apply(inst *Instance, root *Config) error {
	cfg, sub, err := n.DevNet.addSubNode(inst, root, fmt.Sprintf("bvn%d-%d", n.BVN, n.Node))
	if err != nil {
		return err
	}

	cfg.P2P.Key = rawPrivKeyFrom(n.NodeKey)

	// Create partition services
	opts := partOpts{
		CoreValidatorConfiguration: &CoreValidatorConfiguration{
			Listen:       listen(n.DevNet.Listen, devNetDefaultHost, n.IP, portDir),
			ValidatorKey: rawPrivKeyFrom(n.PrivVal),
			StorageType:  Ptr(StorageTypeBadger),
		},
		ID:               protocol.Directory,
		Type:             protocol.PartitionTypeDirectory,
		Dir:              "dnn",
		Genesis:          filepath.Join("..", "dn-genesis.snap"),
		BootstrapPeers:   n.DnBootstrap,
		MetricsNamespace: fmt.Sprintf("consensus_directory_%d", n.IP),
	}
	err = opts.apply(cfg)
	if err != nil {
		return err
	}

	opts.ID = fmt.Sprintf("BVN%d", n.BVN)
	opts.Type = protocol.PartitionTypeBlockValidator
	opts.Dir = "bvnn"
	opts.Genesis = filepath.Join("..", fmt.Sprintf("bvn%d-genesis.snap", n.BVN))
	opts.Listen = listen(n.DevNet.Listen, devNetDefaultHost, n.IP, portBVN)
	opts.MetricsNamespace = fmt.Sprintf("consensus_bvn%d_%d", n.BVN, n.IP)
	opts.BootstrapPeers = n.BvnBootstrap
	err = opts.apply(cfg)
	if err != nil {
		return err
	}

	return n.DevNet.writeSubNode(inst, root, cfg, sub, n.IP)
}

func (d *DevnetConfiguration) addSubNode(inst *Instance, root *Config, name string) (*Config, *SubnodeService, error) {
	sub := addService(root, &SubnodeService{Name: name}, func(s *SubnodeService) string { return s.Name })
	cfg := &Config{Services: sub.Services}
	cfg.file = inst.path(sub.Name, "accumulate.toml")
	cfg.P2P = new(P2P)

	err := os.MkdirAll(inst.path(sub.Name), 0700)
	if err != nil {
		return nil, nil, err
	}

	return cfg, sub, nil
}

func (d *DevnetConfiguration) writeSubNode(inst *Instance, root, cfg *Config, sub *SubnodeService, ip ipOffset) error {
	// Update the subnode configuration
	sub.NodeKey = cfg.P2P.Key
	sub.Services = cfg.Services

	// Copy from the root config
	cfg.Network = root.Network
	cfg.Logging = root.Logging

	// P2P listening addresses
	cfg.P2P.Listen = []multiaddr.Multiaddr{
		listen(d.Listen, devNetDefaultHost, ip, portDir+portAccP2P, useTCP{}),
		listen(d.Listen, devNetDefaultHost, ip, portDir+portAccP2P, useQUIC{}),
		listen(d.Listen, devNetDefaultHost, ip, portBVN+portAccP2P, useTCP{}),
		listen(d.Listen, devNetDefaultHost, ip, portBVN+portAccP2P, useQUIC{}),
	}

	// Sub-nodes are bootstrapped from the bootstrap node
	if ip > 0 {
		cfg.P2P.BootstrapPeers = []multiaddr.Multiaddr{
			listen(d.Listen, devNetDefaultHost, portDir+portAccP2P, useTCP{}),
		}
	}

	// Write out the sub-node config
	return cfg.Save()
}
