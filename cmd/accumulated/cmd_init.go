// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	cfg "gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/proxy"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdInit = &cobra.Command{
	Use:   "init",
	Short: "Initialize a network or node",
	Args:  cobra.NoArgs,
}

var cmdInitNode = &cobra.Command{
	Use:   "node <partition.network name> or <peer url>",
	Short: "Initialize a node using the partition.network name and --seed or via a peer URL; it runs --partition, or every partition the peer runs",
	Run: func(cmd *cobra.Command, args []string) {
		out, err := initNode(cmd, args)
		printOutput(cmd, out, err)
	},
	Args: cobra.ExactArgs(1),
}

var flagInit struct {
	Reset           bool
	LogLevels       string
	FactomAddresses string
	Snapshots       []string
	FaucetSeed      string
}

var flagInitNode struct {
	GenesisDoc       string
	ListenIP         string
	PublicIP         string
	Follower         bool
	SkipVersionCheck bool
	SeedProxy        string
	NoPrometheus     bool
	Partition        string
}

var flagInitDualNode struct {
	Follower         bool
	SkipVersionCheck bool
	SeedProxy        string
	PublicIP         string
	//ResolvePublicIP  bool
	NoPrometheus bool
	ListenIP     string

	DnGenesis  string
	BvnGenesis string
}

var flagInitNetwork struct {
	GenesisDoc     string
	FactomBalances string
	Database       string
	BlockInterval  time.Duration
}

func initInitFlags() {
	cmdInitNetwork.ResetFlags()
	cmdInitNetwork.Flags().StringVar(&flagInitNetwork.GenesisDoc, "genesis-doc", "", "Genesis doc for the target network")
	cmdInitNetwork.Flags().StringVar(&flagInitNetwork.Database, "database", "", "Storage backend to pin in every generated node configuration (default: the binary's default)")
	cmdInitNetwork.Flags().StringVar(&flagInitNetwork.FactomBalances, "factom-balances", "", "Factom addresses and balances file path for writing onto the genesis block")
	cmdInitNetwork.Flags().DurationVar(&flagInitNetwork.BlockInterval, "block-interval", 0, "Target time between blocks, pinned in every generated node configuration (default: the binary's default)")

	cmdInit.ResetFlags()
	cmdInit.PersistentFlags().BoolVar(&flagInit.Reset, "reset", false, "Delete any existing directories within the working directory")
	cmdInit.PersistentFlags().StringVar(&flagInit.LogLevels, "log-levels", "", "Override the default log levels")
	cmdInit.PersistentFlags().StringVar(&flagInit.FactomAddresses, "factom-addresses", "", "A text file containing Factoid addresses to import")
	cmdInit.PersistentFlags().StringSliceVar(&flagInit.Snapshots, "snapshot", nil, "A snapshot of accounts to import")
	cmdInit.PersistentFlags().StringVar(&flagInit.FaucetSeed, "faucet-seed", "", "If specified, generates a faucet account using the given seed and includes it in genesis")
	_ = cmdInit.MarkFlagRequired("network")

	cmdInitNode.ResetFlags()
	cmdInitNode.Flags().BoolVarP(&flagInitNode.Follower, "follow", "f", false, "Do not participate in voting")
	cmdInitNode.Flags().StringVar(&flagInitNode.GenesisDoc, "genesis-doc", "", "Genesis snapshot for the partition, instead of fetching it from the peer (e.g. a <partition>.snap from 'init genesis')")
	cmdInitNode.Flags().StringVarP(&flagInitNode.ListenIP, "listen", "l", "", "Address and port to listen on, e.g. tcp://1.2.3.4:5678, default will be http://0.0.0.0:basePort where base port is determined by seed or peer")
	cmdInitNode.Flags().StringVarP(&flagInitNode.PublicIP, "public", "p", "", "public IP or URL")
	cmdInitNode.Flags().BoolVar(&flagInitNode.SkipVersionCheck, "skip-version-check", false, "Do not enforce the version check")
	cmdInitNode.Flags().StringVar(&flagInitNode.SeedProxy, "seed", "", "Fetch network configuration from seed proxy")
	cmdInitNode.Flags().BoolVarP(&flagInitNode.NoPrometheus, "no-prometheus", "", false, "disable prometheus")
	cmdInitNode.Flags().StringVar(&flagInitNode.Partition, "partition", "", "The partition the node runs, Directory or a BVN's ID (default: every partition the peer runs, its Directory and its BVN)")

	cmdInitDualNode.ResetFlags()
	cmdInitDualNode.Flags().BoolVarP(&flagInitDualNode.Follower, "follow", "f", false, "Do not participate in voting")
	cmdInitDualNode.Flags().BoolVar(&flagInitDualNode.SkipVersionCheck, "skip-version-check", false, "Do not enforce the version check")
	cmdInitDualNode.Flags().StringVarP(&flagInitDualNode.PublicIP, "public", "p", "", "public IP or URL")
	cmdInitDualNode.Flags().StringVarP(&flagInitDualNode.ListenIP, "listen", "l", "", "Address to listen on, where port is determined by seed or peer")
	cmdInitDualNode.Flags().StringVar(&flagInitDualNode.SeedProxy, "seed", "", "Fetch network configuration from seed proxy")
	cmdInitDualNode.Flags().BoolVarP(&flagInitDualNode.NoPrometheus, "no-prometheus", "", false, "disable prometheus")
	//cmdInitDualNode.Flags().BoolVar(&flagInitDualNode.ResolvePublicIP, "resolve-public-ip", true, "resolve public IP address of node and add to configuration")

	cmdInitDualNode.Flags().StringVar(&flagInitDualNode.DnGenesis, "dn-genesis-doc", "", "Genesis doc for the DN")
	cmdInitDualNode.Flags().StringVar(&flagInitDualNode.BvnGenesis, "bvn-genesis-doc", "", "Genesis doc for the target BVN")
}

func init() {
	cmdMain.AddCommand(cmdInit)
	cmdInit.AddCommand(cmdInitNode, cmdInitNetwork, cmdInitDualNode)

	initInitFlags()
}

func networkReset() {
	ent, err := os.ReadDir(flagMain.WorkDir)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	check(err)
	for _, ent := range ent {
		if ent.Name() == "priv_validator_key.json" {
			err := os.Remove(filepath.Join(flagMain.WorkDir, ent.Name()))
			check(err)
		}
		if !ent.IsDir() {
			continue
		}

		dir := filepath.Join(flagMain.WorkDir, ent.Name())
		if strings.HasPrefix(ent.Name(), "dnn") || strings.HasPrefix(ent.Name(), "bvnn") {
			// Delete
		} else if ent.Name() == "bootstrap" {
			if !bootstrapReset(dir) {
				fmt.Printf("Keeping %s\n", dir)
				continue
			}
		} else if !strings.HasPrefix(ent.Name(), "node-") && !strings.HasPrefix(ent.Name(), "bsn-") {
			fmt.Fprintf(os.Stderr, "Skipping %s\n", dir)
			continue
		} else if !nodeReset(dir) {
			fmt.Printf("Keeping %s\n", dir)
			continue
		}

		fmt.Fprintf(os.Stderr, "Deleting %s\n", dir)
		err = os.RemoveAll(dir)
		check(err)
	}
}

func nodeReset(dir string) bool {
	ent, err := os.ReadDir(dir)
	check(err)
	var keep bool
	for _, ent := range ent {
		if ent.Name() == "priv_validator_key.json" {
			file := filepath.Join(dir, ent.Name())
			fmt.Fprintf(os.Stderr, "Deleting %s\n", file)
			err := os.Remove(file)
			check(err)
			continue
		}
		if !ent.IsDir() {
			keep = true
			continue
		}

		switch strings.ToLower(ent.Name()) {
		case "dnn", "bvnn", "bsnn":
			dir := path.Join(dir, ent.Name())
			fmt.Fprintf(os.Stderr, "Deleting %s\n", dir)
			err = os.RemoveAll(dir)
			check(err)

		default:
			dir := path.Join(dir, ent.Name())
			fmt.Fprintf(os.Stderr, "Skipping %s\n", dir)
			keep = true
		}
	}
	return !keep
}

func bootstrapReset(dir string) bool {
	ent, err := os.ReadDir(dir)
	check(err)
	var keep bool
	for _, ent := range ent {
		switch ent.Name() {
		case "node_key.json", "accumulate.toml":
			file := filepath.Join(dir, ent.Name())
			fmt.Fprintf(os.Stderr, "Deleting %s\n", file)
			err := os.Remove(file)
			check(err)
			continue

		default:
			dir := path.Join(dir, ent.Name())
			fmt.Fprintf(os.Stderr, "Skipping %s\n", dir)
			keep = true
		}
		if !ent.IsDir() {
			keep = true
			continue
		}
	}
	return !keep
}

// joinInit is what `init node` and `init dual` learn from the network: enough
// to write a node configuration of the same layout `init network` writes
// (network spec, "How a new node gets its genesis").
type joinInit struct {
	network   string
	basePort  int
	bootstrap []multiaddr.Multiaddr

	dn         bool   // the node runs the Directory
	dnGenesis  []byte // the Directory's genesis snapshot
	bvn        string // the BVN the node runs, if any
	bvnGenesis []byte // the BVN's genesis snapshot
}

// seedPeer asks a seed proxy for the partition's peers, checks that the proxy
// is registered and its answers are signed, and returns a peer's address and
// the partition's ID.
func seedPeer(ctx context.Context, arg string) (string, string, error) {
	s := strings.Split(arg, ".")
	if len(s) != 2 {
		return "", "", fmt.Errorf("network must be in the form of <partition-name>.<network-name>, e.g. bvn0.mainnet")
	}
	partitionName := s[0]
	networkName := s[1]

	seedProxy, err := proxy.New(flagInitNode.SeedProxy)
	if err != nil {
		return "", "", err
	}

	slr := proxy.SeedListRequest{}
	slr.Network = networkName
	slr.Partition = partitionName
	slr.Sign = true
	resp, err := seedProxy.GetSeedList(ctx, &slr)
	if err != nil {
		return "", "", fmt.Errorf("proxy returned seeding error, %v", err)
	}

	b, err := resp.SeedList.MarshalBinary()
	if err != nil {
		return "", "", fmt.Errorf("invalid seed list, %v", err)
	}
	if !resp.Signature.Verify(nil, protocol.SignableHash(sha256.Sum256(b))) {
		return "", "", fmt.Errorf("invalid signature from proxy")
	}
	if len(resp.Addresses) == 0 {
		return "", "", fmt.Errorf("no peers, cannot continue")
	}

	// The node that answers must know the proxy: its registry key page must
	// hold the key the proxy signed with.
	addr := resp.Addresses[len(resp.Addresses)-1]
	u, err := cfg.OffsetPort(addr, int(resp.BasePort), int(cfg.PortOffsetAccumulateApi))
	if err != nil {
		return "", "", fmt.Errorf("failed to parse url from network info %s, %v", addr, err)
	}
	v2, err := client.New(fmt.Sprintf("http://%s:%s", u.Hostname(), u.Port()))
	if err != nil {
		return "", "", fmt.Errorf("failed to create accumulate client for %s, %v", u.String(), err)
	}
	kp := new(protocol.KeyPage)
	res := new(api.ChainQueryResponse)
	res.Data = kp
	req := new(api.GeneralQuery)
	req.Url = protocol.AccountUrl("accuproxy.acme").JoinPath("registry", "1")
	err = v2.RequestAPIv2(ctx, "query", req, res)
	if err != nil {
		return "", "", fmt.Errorf("cannot query the accuproxy registry, %v", err)
	}
	if _, _, found := kp.EntryByKeyHash(resp.Signature.GetPublicKeyHash()); !found {
		return "", "", fmt.Errorf("seed proxy is not registered")
	}

	ncr := proxy.NetworkConfigRequest{}
	ncr.Sign = true
	ncr.Network = networkName
	nc, err := seedProxy.GetNetworkConfig(ctx, &ncr)
	if err != nil {
		return "", "", err
	}
	d, err := nc.NetworkState.MarshalBinary()
	if err != nil {
		return "", "", err
	}
	if !nc.Signature.Verify(nil, protocol.SignableHash(sha256.Sum256(d))) {
		return "", "", fmt.Errorf("cannot verify network configuration from proxy")
	}
	if _, _, found := kp.EntryByKeyHash(nc.Signature.GetPublicKeyHash()); !found {
		return "", "", fmt.Errorf("seed proxy is not registered")
	}
	err = versionCheck(&api.VersionResponse{
		Version:        nc.NetworkState.Version,
		Commit:         nc.NetworkState.Commit,
		VersionIsKnown: nc.NetworkState.VersionIsKnown,
		IsTestNet:      nc.NetworkState.IsTestNet,
	}, "proxy")
	if err != nil {
		return "", "", err
	}

	partition := partitionName
	if resp.Type == protocol.PartitionTypeDirectory {
		partition = protocol.Directory
	}
	return fmt.Sprintf("tcp://%s:%d", u.Hostname(), resp.BasePort), partition, nil
}

// joinFromPeer asks a running DAG-BFT node, through API v3, for everything a
// new node needs: the network, the peer's libp2p identity (its bootstrap
// peer), the partition(s) to run and their genesis snapshots. A node has one
// API, at its base port plus the API offset, whichever partition is asked for.
//
// The node runs the partitions named, or with none named, every partition the
// peer runs: its Directory and its BVN, the layout `init dual` writes. Each is
// confirmed by asking the peer's consensus service for that partition, so one
// the peer does not run is refused here rather than when the node fails to
// join.
func joinFromPeer(ctx context.Context, peer string, partitions []string, dnGenesisFile, bvnGenesisFile string) (*joinInit, error) {
	netAddr, netPort, err := resolveAddrWithPort(peer)
	if err != nil {
		return nil, fmt.Errorf("invalid peer url %v", err)
	}

	accClient := jsonrpc.NewClient(fmt.Sprintf("http://%s:%d/v3", netAddr, netPort+int(cfg.PortOffsetAccumulateApi)))
	ni, err := accClient.NodeInfo(ctx, v3.NodeInfoOptions{})
	if err != nil {
		return nil, err
	}

	err = versionCheck(&api.VersionResponse{
		Version:        ni.Version,
		Commit:         ni.Commit,
		VersionIsKnown: ni.Commit != "",
	}, peer)
	if err != nil {
		return nil, err
	}

	// The peer is the new node's bootstrap peer, named by the libp2p identity
	// its node info reports, on its one P2P port.
	peerAddr, err := peerMultiaddr(netAddr, netPort+int(cfg.PortOffsetAccumulateP2P), ni.PeerID.String())
	if err != nil {
		return nil, fmt.Errorf("bootstrap peer for %s: %v", peer, err)
	}

	j := &joinInit{
		network:   ni.Network,
		basePort:  netPort,
		bootstrap: []multiaddr.Multiaddr{peerAddr},
	}

	if len(partitions) == 0 {
		partitions = partitionsOf(ni.Services)
		if len(partitions) == 0 {
			return nil, fmt.Errorf("%s runs no partition", peer)
		}
	}
	for _, id := range partitions {
		cs, err := accClient.ConsensusStatus(ctx, v3.ConsensusStatusOptions{NodeID: ni.PeerID.String(), Partition: id})
		if err != nil {
			return nil, fmt.Errorf("%s does not serve partition %s: %v", peer, id, err)
		}
		if cs.PartitionID != "" {
			id = cs.PartitionID
		}
		switch cs.PartitionType {
		case protocol.PartitionTypeDirectory:
			j.dn = true
		case protocol.PartitionTypeBlockValidator:
			if j.bvn != "" && !strings.EqualFold(j.bvn, id) {
				return nil, fmt.Errorf("a node runs one BVN, not %s and %s", j.bvn, id)
			}
			j.bvn = id
		default:
			return nil, fmt.Errorf("%s: unsupported partition type %v", id, cs.PartitionType)
		}
	}

	// Genesis comes from the peer's own consensus service for each partition
	// (network spec, "How a new node gets its genesis").
	if j.dn {
		j.dnGenesis, err = getGenesis(ctx, peer, accClient, v3.GenesisOptions{NodeID: ni.PeerID.String(), Partition: protocol.Directory}, dnGenesisFile)
		if err != nil {
			return nil, err
		}
	}
	if j.bvn != "" {
		j.bvnGenesis, err = getGenesis(ctx, peer, accClient, v3.GenesisOptions{NodeID: ni.PeerID.String(), Partition: j.bvn}, bvnGenesisFile)
		if err != nil {
			return nil, err
		}
	}
	return j, nil
}

// getGenesis returns the genesis snapshot for the partition: the named file if
// there is one, otherwise the peer's copy, asked for through API v3's genesis
// method.
func getGenesis(ctx context.Context, server string, peer v3.GenesisService, opts v3.GenesisOptions, file string) ([]byte, error) {
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to load genesis doc %q, %v", file, err)
		}
		return b, nil
	}

	warnf("You are fetching the Genesis document from %s! Only do this if you trust %[1]s and your connection to it!", server)
	b, err := peer.Genesis(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get genesis for %s from %s, %v", opts.Partition, server, err)
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("%s returned an empty genesis for %s", server, opts.Partition)
	}
	return b, nil
}

// peerMultiaddr is /ip4|ip6|dns/{host}/tcp/{port}/p2p/{id}.
func peerMultiaddr(host string, port int, id string) (multiaddr.Multiaddr, error) {
	return hostMultiaddr(host, fmt.Sprintf("/tcp/%d/p2p/%s", port, id))
}

// hostMultiaddr is /ip4|ip6|dns/{host}{rest}.
func hostMultiaddr(host, rest string) (multiaddr.Multiaddr, error) {
	proto := "dns"
	if ip := net.ParseIP(host); ip != nil {
		proto = "ip6"
		if ip.To4() != nil {
			proto = "ip4"
		}
	}
	return multiaddr.NewMultiaddr(fmt.Sprintf("/%s/%s%s", proto, host, rest))
}

// nodeInitOptions are the operator's choices for the node `init node` and
// `init dual` write.
type nodeInitOptions struct {
	Listen       string // address (and optionally port) to listen on
	Public       string // address other nodes reach this one by
	Follower     bool   // follow, do not validate
	NoPrometheus bool
}

// writeNodeConfig writes the node's accumulate.toml and genesis snapshots into
// the work directory, in the layout `init network` writes for each node: a
// core validator (or follower) configuration with its keys inline, joining a
// running network through the peer it was initialized from. It generates the
// node's keys; it never overwrites a configuration that exists.
func writeNodeConfig(dir string, j *joinInit, opts nodeInitOptions) error {
	file := filepath.Join(dir, "accumulate.toml")
	if _, err := os.Stat(file); err == nil {
		return fmt.Errorf("%s already exists; use --reset to replace it", file)
	}
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	host, base := "0.0.0.0", j.basePort
	if opts.Listen != "" {
		u, err := url.Parse(opts.Listen)
		if err != nil || u.Hostname() == "" {
			return fmt.Errorf("invalid --listen %q", opts.Listen)
		}
		host = u.Hostname()
		if u.Port() != "" {
			p, err := strconv.ParseUint(u.Port(), 10, 16)
			if err != nil {
				return fmt.Errorf("invalid port number %q, %v", u.Port(), err)
			}
			base = int(p)
		}
	}
	listen, err := hostMultiaddr(host, fmt.Sprintf("/tcp/%d", base))
	if err != nil {
		return fmt.Errorf("listen address: %v", err)
	}

	c := new(run.Config)
	c.Network = j.network
	c.P2P = new(run.P2P)
	c.P2P.BootstrapPeers = j.bootstrap
	nodeKey, err := newKey()
	if err != nil {
		return err
	}
	c.P2P.Key = nodeKey
	if opts.Public != "" {
		public, err := resolveIp(opts.Public)
		if err != nil {
			return fmt.Errorf("invalid public address %v", err)
		}
		c.P2P.External, err = hostMultiaddr(public, fmt.Sprintf("/tcp/%d", base+int(cfg.PortOffsetAccumulateP2P)))
		if err != nil {
			return fmt.Errorf("public address: %v", err)
		}
	}
	if !opts.NoPrometheus {
		prom, err := hostMultiaddr(host, fmt.Sprintf("/tcp/%d/http", base+int(cfg.PortOffsetPrometheus)))
		if err != nil {
			return err
		}
		c.Instrumentation = &run.Instrumentation{HttpListener: run.HttpListener{Listen: []run.Multiaddr{prom}}}
	}

	var mode run.CoreValidatorMode
	switch {
	case j.dn && j.bvn != "":
		mode = run.CoreValidatorModeDual
	case j.dn:
		mode = run.CoreValidatorModeDN
	default:
		mode = run.CoreValidatorModeBVN
	}

	var dnGenesis, bvnGenesis string
	if j.dn {
		dnGenesis = "directory-genesis.snap"
		err = os.WriteFile(filepath.Join(dir, dnGenesis), j.dnGenesis, 0600)
		if err != nil {
			return err
		}
	}
	if j.bvn != "" {
		bvnGenesis = strings.ToLower(j.bvn) + "-genesis.snap"
		err = os.WriteFile(filepath.Join(dir, bvnGenesis), j.bvnGenesis, 0600)
		if err != nil {
			return err
		}
	}

	if opts.Follower {
		c.Configurations = []run.Configuration{&run.FollowerConfiguration{
			Mode:              mode,
			Listen:            listen,
			BVN:               j.bvn,
			DnGenesis:         dnGenesis,
			BvnGenesis:        bvnGenesis,
			DnBootstrapPeers:  j.bootstrap,
			BvnBootstrapPeers: j.bootstrap,
		}}
	} else {
		valKey, err := newKey()
		if err != nil {
			return err
		}
		c.Configurations = []run.Configuration{&run.CoreValidatorConfiguration{
			Mode:               mode,
			Listen:             listen,
			BVN:                j.bvn,
			ValidatorKey:       valKey,
			DnGenesis:          dnGenesis,
			BvnGenesis:         bvnGenesis,
			DnBootstrapPeers:   j.bootstrap,
			BvnBootstrapPeers:  j.bootstrap,
			JoinRunningNetwork: run.Ptr(true),
		}}
	}

	return c.SaveTo(file)
}

// newKey generates an ed25519 key, written inline as `init network` writes
// its nodes' keys.
func newKey() (*run.RawPrivateKey, error) {
	_, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &run.RawPrivateKey{Address: address.FromED25519PrivateKey(sk).String()}, nil
}

// resetNodeDir deletes what `init node` or `init dual` wrote into dir: the
// configuration, the genesis snapshots and the partitions' data.
func resetNodeDir(dir string) {
	ents, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	check(err)
	for _, ent := range ents {
		name := ent.Name()
		if name == "accumulate.toml" || strings.HasSuffix(name, "-genesis.snap") || name == "dnn" || name == "bvnn" {
			fmt.Fprintf(os.Stderr, "Deleting %s\n", filepath.Join(dir, name))
			check(os.RemoveAll(filepath.Join(dir, name)))
		}
	}
}

func initNode(cmd *cobra.Command, args []string) (string, error) {
	if !cmd.Flag("work-dir").Changed {
		return cmd.UsageString(), fmt.Errorf("Error: --work-dir flag is required\n\n")
	}

	peer := args[0]
	var partitions []string
	if flagInitNode.Partition != "" {
		partitions = []string{flagInitNode.Partition}
	}
	if flagInitNode.SeedProxy != "" {
		var err error
		var partition string
		peer, partition, err = seedPeer(cmd.Context(), args[0])
		if err != nil {
			return "", fmt.Errorf("failed to configure node from seed proxy, %v", err)
		}
		if partitions == nil {
			partitions = []string{partition}
		}
	}

	j, err := joinFromPeer(cmd.Context(), peer, partitions, flagInitNode.GenesisDoc, flagInitNode.GenesisDoc)
	if err != nil {
		return "", fmt.Errorf("failed to configure node from peer, %v", err)
	}

	if flagInit.Reset {
		resetNodeDir(flagMain.WorkDir)
	}

	err = writeNodeConfig(flagMain.WorkDir, j, nodeInitOptions{
		Listen:       flagInitNode.ListenIP,
		Public:       flagInitNode.PublicIP,
		Follower:     flagInitNode.Follower,
		NoPrometheus: flagInitNode.NoPrometheus,
	})
	if err != nil {
		return "", fmt.Errorf("write node files, %v", err)
	}
	return "", nil
}

func newLogger() logging.Logger {
	levels := cfg.DefaultLogLevels
	if flagInit.LogLevels != "" {
		levels = flagInit.LogLevels
	}

	writer, err := logging.NewConsoleWriter("plain")
	check(err)
	level, writer, err := logging.ParseLogLevel(levels, writer)
	check(err)
	logger, err := logging.NewZerologLogger(zerolog.New(writer), level, false)
	check(err)
	return logger
}

func resolveIp(addr string) (string, error) {
	host, err := resolveAddr(addr)
	if err == nil {
		return host, nil
	}

	ip := net.ParseIP(addr)
	if ip == nil {
		return "", fmt.Errorf("address in not an IP or url")
	}

	return ip.String(), nil
}

func resolveAddrWithPort(addr string) (string, int, error) {
	ip, err := url.Parse(addr)
	if err != nil {
		//try adding a scheme to see if that helps
		ip, err = url.Parse("tcp://" + addr)
		if err != nil {
			return "", 0, fmt.Errorf("%q is not a URL", addr)
		}
	}

	if ip.Path != "" && ip.Path != "/" {
		return "", 0, fmt.Errorf("address cannot have a path")
	}

	if ip.Port() == "" {
		return "", 0, fmt.Errorf("%q does not specify a port number", addr)
	}
	port, err := strconv.ParseUint(ip.Port(), 10, 16)
	if err != nil {
		return "", 0, fmt.Errorf("%q is not a valid port number", ip.Port())
	}

	return ip.Hostname(), int(port), nil
}

func resolveAddr(addr string) (string, error) {
	ip, err := url.Parse(addr)
	if err != nil {
		//try adding a scheme to see if that helps
		ip, err = url.Parse("tcp://" + addr)
		if err != nil {
			return "", fmt.Errorf("%q is not a URL", addr)
		}
	}

	if ip.Path != "" && ip.Path != "/" {
		return "", fmt.Errorf("address cannot have a path")
	}

	return ip.Hostname(), nil
}

func versionCheck(version *api.VersionResponse, peer string) error {
	switch {
	case !accumulate.IsVersionKnown() && !version.VersionIsKnown:
		warnf("The version of this executable and %s is unknown. If there is a version mismatch, the node may fail.", peer)

	case accumulate.Commit != version.Commit:
		if flagInitNode.SkipVersionCheck {
			warnf("This executable is version %s but %s is %s. This may cause the node to fail.", formatVersion(accumulate.Version, accumulate.IsVersionKnown()), peer, formatVersion(version.Version, version.VersionIsKnown))
		} else {
			return fmt.Errorf("wrong version: network is %s, we are %s", formatVersion(version.Version, version.VersionIsKnown), formatVersion(accumulate.Version, accumulate.IsVersionKnown()))
		}
	}
	return nil
}

// partitionsOf names the partitions a node runs, from the consensus services
// its node info lists: the Directory, if it runs it, then its BVN.
func partitionsOf(services []*v3.ServiceAddress) []string {
	var ids []string
	for _, svc := range services {
		if svc.Type == v3.ServiceTypeConsensus && strings.EqualFold(svc.Argument, protocol.Directory) {
			ids = append(ids, protocol.Directory)
			break
		}
	}
	if bvn, ok := bvnOf(services); ok {
		ids = append(ids, bvn)
	}
	return ids
}

// bvnOf names the BVN a node runs: the partition of its consensus service
// that is not the Directory's, whatever order the services are listed in.
func bvnOf(services []*v3.ServiceAddress) (string, bool) {
	for _, svc := range services {
		if svc.Type != v3.ServiceTypeConsensus || strings.EqualFold(svc.Argument, protocol.Directory) {
			continue
		}
		return svc.Argument, true
	}
	return "", false
}
