package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/libs/log"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/config"
	cfg "gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/proxy"
	etcd "go.etcd.io/etcd/client/v3"
)

var cmdInit = &cobra.Command{
	Use:   "init",
	Short: "Initialize a network or node",
	Args:  cobra.NoArgs,
}

var cmdInitNode = &cobra.Command{
	Use:   "node <node nr> <network-name|url>",
	Short: "Initialize a node",
	Run:   initNode,
	Args:  cobra.ExactArgs(2),
}

var flagInit struct {
	NoEmptyBlocks bool
	NoWebsite     bool
	Reset         bool
	LogLevels     string
	Etcd          []string
}

var flagInitNode struct {
	GenesisDoc          string
	ListenIP            string
	Follower            bool
	SkipVersionCheck    bool
	SeedProxy           string
	AllowUnhealthyPeers bool
}

var flagInitDualNode struct {
	GenesisDoc       string
	Follower         bool
	SkipVersionCheck bool
	SeedProxy        string
}

var flagInitDevnet struct {
	Name          string
	NumBvns       int
	NumValidators int
	NumFollowers  int
	BasePort      int
	IPs           []string
	Docker        bool
	DockerImage   string
	UseVolumes    bool
	Compose       bool
	DnsSuffix     string
}

var flagInitNetwork struct {
	GenesisDoc     string
	FactomBalances string
}

func init() {
	cmdMain.AddCommand(cmdInit)
	cmdInit.AddCommand(cmdInitNode, cmdInitDevnet, cmdInitNetwork, cmdInitDualNode)

	cmdInitNetwork.Flags().StringVar(&flagInitNetwork.GenesisDoc, "genesis-doc", "", "Genesis doc for the target network")
	cmdInitNetwork.Flags().StringVar(&flagInitNetwork.FactomBalances, "factom-balances", "", "Factom addresses and balances file path for writing onto the genesis block")

	cmdInit.PersistentFlags().BoolVar(&flagInit.NoEmptyBlocks, "no-empty-blocks", false, "Do not create empty blocks")
	cmdInit.PersistentFlags().BoolVar(&flagInit.NoWebsite, "no-website", false, "Disable website")
	cmdInit.PersistentFlags().BoolVar(&flagInit.Reset, "reset", false, "Delete any existing directories within the working directory")
	cmdInit.PersistentFlags().StringVar(&flagInit.LogLevels, "log-levels", "", "Override the default log levels")
	cmdInit.PersistentFlags().StringSliceVar(&flagInit.Etcd, "etcd", nil, "Use etcd endpoint(s)")
	_ = cmdInit.MarkFlagRequired("network")

	cmdInitNode.Flags().BoolVarP(&flagInitNode.Follower, "follow", "f", false, "Do not participate in voting")
	cmdInitNode.Flags().StringVar(&flagInitNode.GenesisDoc, "genesis-doc", "", "Genesis doc for the target network")
	cmdInitNode.Flags().StringVarP(&flagInitNode.ListenIP, "listen", "l", "", "Address and port to listen on, e.g. tcp://1.2.3.4:5678")
	cmdInitNode.Flags().BoolVar(&flagInitNode.SkipVersionCheck, "skip-version-check", false, "Do not enforce the version check")
	cmdInitNode.Flags().StringVar(&flagInitNode.SeedProxy, "seed", "", "Fetch network configuration from seed proxy")
	cmdInitNode.Flags().BoolVarP(&flagInitNode.AllowUnhealthyPeers, "skip-peer-health-check", "", false, "do not check health of peers")
	_ = cmdInitNode.MarkFlagRequired("listen")

	cmdInitDualNode.Flags().BoolVarP(&flagInitDualNode.Follower, "follow", "f", false, "Do not participate in voting")
	cmdInitDualNode.Flags().StringVar(&flagInitDualNode.GenesisDoc, "genesis-doc", "", "Genesis doc for the target network")
	cmdInitDualNode.Flags().BoolVar(&flagInitDualNode.SkipVersionCheck, "skip-version-check", false, "Do not enforce the version check")
	cmdInitDualNode.Flags().StringVar(&flagInitDualNode.SeedProxy, "seed", "", "Fetch network configuration from seed proxy")

	cmdInitDevnet.Flags().StringVar(&flagInitDevnet.Name, "name", "DevNet", "Network name")
	cmdInitDevnet.Flags().IntVarP(&flagInitDevnet.NumBvns, "bvns", "b", 2, "Number of block validator networks to configure")
	cmdInitDevnet.Flags().IntVarP(&flagInitDevnet.NumValidators, "validators", "v", 2, "Number of validator nodes per subnet to configure")
	cmdInitDevnet.Flags().IntVarP(&flagInitDevnet.NumFollowers, "followers", "f", 1, "Number of follower nodes per subnet to configure")
	cmdInitDevnet.Flags().IntVar(&flagInitDevnet.BasePort, "port", 26656, "Base port to use for listeners")
	cmdInitDevnet.Flags().StringSliceVar(&flagInitDevnet.IPs, "ip", []string{"127.0.1.1"}, "IP addresses to use or base IP - must not end with .0")
	cmdInitDevnet.Flags().BoolVar(&flagInitDevnet.Docker, "docker", false, "Configure a network that will be deployed with Docker Compose")
	cmdInitDevnet.Flags().StringVar(&flagInitDevnet.DockerImage, "image", "registry.gitlab.com/accumulatenetwork/accumulate", "Docker image name (and tag)")
	cmdInitDevnet.Flags().BoolVar(&flagInitDevnet.UseVolumes, "use-volumes", false, "Use Docker volumes instead of a local directory")
	cmdInitDevnet.Flags().BoolVar(&flagInitDevnet.Compose, "compose", false, "Only write the Docker Compose file, do not write the configuration files")
	cmdInitDevnet.Flags().StringVar(&flagInitDevnet.DnsSuffix, "dns-suffix", "", "DNS suffix to add to hostnames used when initializing dockerized nodes")
}

func nodeReset() {
	ent, err := os.ReadDir(flagMain.WorkDir)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	check(err)

	for _, ent := range ent {
		if !ent.IsDir() {
			continue
		}
		isDelete := false
		if strings.HasPrefix(ent.Name(), "bvn") {
			isDelete = true
		} else if ent.Name() == "dn" {
			isDelete = true
		} else if strings.HasSuffix(ent.Name(), ".db") {
			isDelete = true
		}
		if isDelete {
			dir := path.Join(flagMain.WorkDir, ent.Name())
			fmt.Fprintf(os.Stderr, "Deleting %s\n", dir)
			err = os.RemoveAll(dir)
			check(err)
		} else {
			dir := path.Join(flagMain.WorkDir, ent.Name())
			fmt.Fprintf(os.Stderr, "Skipping %s\n", dir)
		}
	}
}

func initNode(cmd *cobra.Command, args []string) {
	nodeDir := args[0]
	nodeNr, err := strconv.ParseUint(nodeDir, 10, 16)
	if err == nil {
		nodeDir = fmt.Sprintf("node-%d", nodeNr)
	}

	netAddr, netPort, err := resolveAddr(args[1])
	checkf(err, "invalid network URL")

	u, err := url.Parse(flagInitNode.ListenIP)
	checkf(err, "invalid --listen %q", flagInitNode.ListenIP)

	nodePort := 26656
	if u.Port() != "" {
		p, err := strconv.ParseInt(u.Port(), 10, 16)
		if err != nil {
			fatalf("invalid port number %q", u.Port())
		}
		nodePort = int(p)
	}

	accClient, err := client.New(fmt.Sprintf("http://%s:%d", netAddr, netPort+int(config.PortOffsetAccumulateApi)))
	checkf(err, "failed to create API client for %s", args[0])

	tmClient, err := rpchttp.New(fmt.Sprintf("tcp://%s:%d", netAddr, netPort+int(config.PortOffsetTendermintRpc)))
	checkf(err, "failed to create Tendermint client for %s", args[0])

	version := getVersion(accClient)
	switch {
	case !accumulate.IsVersionKnown() && !version.VersionIsKnown:
		warnf("The version of this executable and %s is unknown. If there is a version mismatch, the node may fail.", args[0])

	case accumulate.Commit != version.Commit:
		if flagInitNode.SkipVersionCheck {
			warnf("This executable is version %s but %s is %s. This may cause the node to fail.", formatVersion(accumulate.Version, accumulate.IsVersionKnown()), args[0], formatVersion(version.Version, version.VersionIsKnown))
		} else {
			fatalf("wrong version: network is %s, we are %s", formatVersion(version.Version, version.VersionIsKnown), formatVersion(accumulate.Version, accumulate.IsVersionKnown()))
		}
	}

	description, err := accClient.Describe(context.Background())
	checkf(err, "failed to get description from %s", args[0])

	var genDoc *types.GenesisDoc
	if cmd.Flag("genesis-doc").Changed {
		genDoc, err = types.GenesisDocFromFile(flagInitNode.GenesisDoc)
		checkf(err, "failed to load genesis doc %q", flagInitNode.GenesisDoc)
	} else {
		warnf("You are fetching the Genesis document from %s! Only do this if you trust %[1]s and your connection to it!", args[0])
		rgen, err := tmClient.Genesis(context.Background())
		checkf(err, "failed to get genesis from %s", args[0])
		genDoc = rgen.Genesis
	}

	status, err := tmClient.Status(context.Background())
	checkf(err, "failed to get status of %s", args[0])

	nodeType := cfg.Validator
	if flagInitNode.Follower {
		nodeType = cfg.Follower
	}
	config := config.Default(description.Network.Id, description.NetworkType, nodeType, description.SubnetId)
	config.P2P.BootstrapPeers = fmt.Sprintf("%s@%s:%d", status.NodeInfo.NodeID, netAddr, netPort+int(cfg.PortOffsetTendermintP2P))

	if flagInitNode.SeedProxy != "" {
		if flagInitNode.AllowUnhealthyPeers {
			warnf("peers must be checked to use for bootstrapping when using, --allow-unhealthy-peers will have no effect")
		}
		//go gather a more robust network description
		seedProxy, err := proxy.New(flagInitNode.SeedProxy)
		check(err)
		slr := proxy.SeedListRequest{}
		slr.Network = description.Network.Id
		slr.Subnet = description.SubnetId
		resp, err := seedProxy.GetSeedList(context.Background(), &slr)
		if err != nil {
			checkf(err, "proxy returned seeding error")
		}
		for _, addr := range resp.Addresses {
			//go build a list of healthy nodes
			u, err := cfg.OffsetPort(addr, netPort, int(cfg.PortOffsetTendermintP2P))
			checkf(err, "failed to parse url from network info %s", addr)

			//check the health of the peer
			peerClient, err := rpchttp.New(fmt.Sprintf("tcp://%s:%s", u.Hostname(), u.Port()))
			checkf(err, "failed to create Tendermint client for %s", u.String())

			peerStatus, err := peerClient.Status(context.Background())
			if err != nil {
				warnf("ignoring peer: not healthy %s", u.String())
				continue
			}

			//if we have a healthy node with a matching id, add it as a bootstrap peer
			config.P2P.BootstrapPeers += "," + peerStatus.NodeInfo.NodeID.AddressString(strconv.Itoa(netPort+int(cfg.PortOffsetTendermintP2P)))
		}
	} else {
		//otherwise make the best out of what we have to establish our bootstrap peers
		netInfo, err := tmClient.NetInfo(context.Background())
		checkf(err, "failed to get network info from node")

		for _, peer := range netInfo.Peers {
			u, err := url.Parse(peer.URL)
			checkf(err, "failed to parse url from network info %s", peer.URL)

			clientUrl := fmt.Sprintf("tcp://%s:%s", u.Hostname(), u.Port())

			if !flagInitNode.AllowUnhealthyPeers {
				//check the health of the peer
				peerClient, err := rpchttp.New(clientUrl)
				checkf(err, "failed to create Tendermint client for %s", u.String())

				peerStatus, err := peerClient.Status(context.Background())
				if err != nil {
					warnf("ignoring peer: not healthy %s", clientUrl)
					continue
				}

				statBytes, err := peerStatus.NodeInfo.NodeID.Bytes()
				if err != nil {
					warnf("ignoring healthy peer %s because peer id is invalid", u.String())
					continue
				}

				peerBytes, err := peer.ID.Bytes()
				if err != nil {
					warnf("ignoring peer %s because node id is not valid", u.String())
					continue
				}

				if bytes.Compare(statBytes, peerBytes) != 0 {
					warnf("ignoring stale peer %s", u.String())
					continue

				}
			}

			//if we have a healthy node with a matching id, add it as a bootstrap peer
			config.P2P.BootstrapPeers += "," + u.String()
		}
	}

	config.Accumulate.Describe = cfg.Describe{NetworkType: description.NetworkType, SubnetId: description.SubnetId, LocalAddress: ""}

	if flagInit.LogLevels != "" {
		_, _, err := logging.ParseLogLevel(flagInit.LogLevels, io.Discard)
		checkf(err, "--log-level")
		config.LogLevel = flagInit.LogLevels
	}

	if len(flagInit.Etcd) > 0 {
		config.Accumulate.Storage.Type = cfg.EtcdStorage
		config.Accumulate.Storage.Etcd = new(etcd.Config)
		config.Accumulate.Storage.Etcd.Endpoints = flagInit.Etcd
		config.Accumulate.Storage.Etcd.DialTimeout = 5 * time.Second
	}

	if flagInit.Reset {
		nodeReset()
	}

	config.SetRoot(filepath.Join(flagMain.WorkDir, nodeDir))
	accumulated.ConfigureNodePorts(&accumulated.NodeInit{
		HostName: u.Hostname(),
		ListenIP: u.Hostname(),
		BasePort: uint64(nodePort),
	}, config, 0)

	// TODO Check for existing?
	privValKey, nodeKey := ed25519.GenPrivKey(), ed25519.GenPrivKey()

	err = accumulated.WriteNodeFiles(config, privValKey, nodeKey, genDoc)
	checkf(err, "write node files")
}

func newLogger() log.Logger {
	levels := config.DefaultLogLevels
	if flagInit.LogLevels != "" {
		levels = flagInit.LogLevels
	}

	writer, err := logging.NewConsoleWriter("plain")
	check(err)
	level, writer, err := logging.ParseLogLevel(levels, writer)
	check(err)
	logger, err := logging.NewTendermintLogger(zerolog.New(writer), level, false)
	check(err)
	return logger
}

func resolveAddr(addr string) (string, int, error) {
	ip, err := url.Parse(addr)
	if err != nil {
		return "", 0, fmt.Errorf("%q is not a URL", addr)
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
