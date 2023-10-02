// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cometbft/cometbft/libs/log"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/cometbft/cometbft/types"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	cfg "gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/proxy"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdInit = &cobra.Command{
	Use:   "init",
	Short: "Initialize a network or node",
	Args:  cobra.NoArgs,
}

var cmdInitNode = &cobra.Command{
	Use:   "node <partition.network name> or <peer url>",
	Short: "Initialize a node using the partition.network name and --seed or via a peer URL",
	Run: func(cmd *cobra.Command, args []string) {
		out, err := initNode(cmd, args)
		printOutput(cmd, out, err)
	},
	Args: cobra.ExactArgs(1),
}

var flagInit struct {
	NoEmptyBlocks    bool
	Reset            bool
	LogLevels        string
	EnableTimingLogs bool
	FactomAddresses  string
	Snapshots        []string
	FaucetSeed       string
}

var flagInitNode struct {
	GenesisDoc          string
	ListenIP            string
	PublicIP            string
	Follower            bool
	SkipVersionCheck    bool
	SeedProxy           string
	AllowUnhealthyPeers bool
	NoPrometheus        bool
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

var flagInitDevnet struct {
	Name          string
	NumBvns       int
	NumValidators int
	NumFollowers  int
	NumBsnNodes   int
	BasePort      int
	IPs           []string
	Docker        bool
	DockerImage   string
	UseVolumes    bool
	Compose       bool
	DnsSuffix     string
	Globals       string
}

var flagInitNetwork struct {
	GenesisDoc     string
	FactomBalances string
}

func initInitFlags() {
	cmdInitNetwork.ResetFlags()
	cmdInitNetwork.Flags().StringVar(&flagInitNetwork.GenesisDoc, "genesis-doc", "", "Genesis doc for the target network")
	cmdInitNetwork.Flags().StringVar(&flagInitNetwork.FactomBalances, "factom-balances", "", "Factom addresses and balances file path for writing onto the genesis block")

	cmdInit.ResetFlags()
	cmdInit.PersistentFlags().BoolVar(&flagInit.NoEmptyBlocks, "no-empty-blocks", false, "Do not create empty blocks")
	cmdInit.PersistentFlags().BoolVar(&flagInit.Reset, "reset", false, "Delete any existing directories within the working directory")
	cmdInit.PersistentFlags().StringVar(&flagInit.LogLevels, "log-levels", "", "Override the default log levels")
	cmdInit.PersistentFlags().BoolVar(&flagInit.EnableTimingLogs, "enable-timing-logs", false, "Enable core timing analysis logging")
	cmdInit.PersistentFlags().StringVar(&flagInit.FactomAddresses, "factom-addresses", "", "A text file containing Factoid addresses to import")
	cmdInit.PersistentFlags().StringSliceVar(&flagInit.Snapshots, "snapshot", nil, "A snapshot of accounts to import")
	cmdInit.PersistentFlags().StringVar(&flagInit.FaucetSeed, "faucet-seed", "", "If specified, generates a faucet account using the given seed and includes it in genesis")
	_ = cmdInit.MarkFlagRequired("network")

	cmdInitNode.ResetFlags()
	cmdInitNode.Flags().BoolVarP(&flagInitNode.Follower, "follow", "f", false, "Do not participate in voting")
	cmdInitNode.Flags().StringVar(&flagInitNode.GenesisDoc, "genesis-doc", "", "Genesis doc for the target network")
	cmdInitNode.Flags().StringVarP(&flagInitNode.ListenIP, "listen", "l", "", "Address and port to listen on, e.g. tcp://1.2.3.4:5678, default will be http://0.0.0.0:basePort where base port is determined by seed or peer")
	cmdInitNode.Flags().StringVarP(&flagInitNode.PublicIP, "public", "p", "", "public IP or URL")
	cmdInitNode.Flags().BoolVar(&flagInitNode.SkipVersionCheck, "skip-version-check", false, "Do not enforce the version check")
	cmdInitNode.Flags().StringVar(&flagInitNode.SeedProxy, "seed", "", "Fetch network configuration from seed proxy")
	cmdInitNode.Flags().BoolVarP(&flagInitNode.AllowUnhealthyPeers, "skip-peer-health-check", "", false, "do not check health of peers")
	cmdInitNode.Flags().BoolVarP(&flagInitNode.NoPrometheus, "no-prometheus", "", false, "disable prometheus")

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

	cmdInitDevnet.ResetFlags()
	cmdInitDevnet.Flags().StringVar(&flagInitDevnet.Name, "name", "DevNet", "Network name")
	cmdInitDevnet.Flags().IntVarP(&flagInitDevnet.NumBvns, "bvns", "b", 2, "Number of block validator networks to configure")
	cmdInitDevnet.Flags().IntVarP(&flagInitDevnet.NumValidators, "validators", "v", 2, "Number of validator nodes per partition to configure")
	cmdInitDevnet.Flags().IntVarP(&flagInitDevnet.NumFollowers, "followers", "f", 1, "Number of follower nodes per partition to configure")
	cmdInitDevnet.Flags().IntVarP(&flagInitDevnet.NumBsnNodes, "bsn", "s", 0, "Number of block summary network nodes")
	cmdInitDevnet.Flags().IntVar(&flagInitDevnet.BasePort, "port", 26656, "Base port to use for listeners")
	cmdInitDevnet.Flags().StringSliceVar(&flagInitDevnet.IPs, "ip", []string{"127.0.1.1"}, "IP addresses to use or base IP - must not end with .0")
	cmdInitDevnet.Flags().BoolVar(&flagInitDevnet.Docker, "docker", false, "Configure a network that will be deployed with Docker Compose")
	cmdInitDevnet.Flags().StringVar(&flagInitDevnet.DockerImage, "image", "registry.gitlab.com/accumulatenetwork/accumulate", "Docker image name (and tag)")
	cmdInitDevnet.Flags().BoolVar(&flagInitDevnet.UseVolumes, "use-volumes", false, "Use Docker volumes instead of a local directory")
	cmdInitDevnet.Flags().BoolVar(&flagInitDevnet.Compose, "compose", false, "Only write the Docker Compose file, do not write the configuration files")
	cmdInitDevnet.Flags().StringVar(&flagInitDevnet.DnsSuffix, "dns-suffix", "", "DNS suffix to add to hostnames used when initializing dockerized nodes")
	cmdInit.PersistentFlags().StringVar(&flagInitDevnet.Globals, "globals", "", "Override network globals")
}

func init() {
	cmdMain.AddCommand(cmdInit)
	cmdInit.AddCommand(cmdInitNode, cmdInitDevnet, cmdInitNetwork, cmdInitDualNode)

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

		fmt.Printf("Deleting %s\n", dir)
		err = os.Remove(dir)
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
			fmt.Printf("Deleting %s\n", file)
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
			fmt.Printf("Deleting %s\n", file)
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

func findInDescribe(addr string, partitionId string, d *cfg.Network) (partition *cfg.Partition, node *cfg.Node, err error) {
	if partitionId == "" {
		partitionId = d.Id
	}
	for i, v := range d.Partitions {
		//search for the address.
		partition = &d.Partitions[i]
		if strings.EqualFold(partition.Id, partitionId) {
			for j, n := range v.Nodes {
				nodeAddr, err := resolveAddr(n.Address)
				if err != nil {
					return nil, nil, fmt.Errorf("cannot resolve node address in network describe")
				}
				if nodeAddr == addr {
					node = &v.Nodes[j]
					return partition, node, nil
				}
			}
			return partition, nil, nil
		}
	}
	return nil, nil, fmt.Errorf("cannot locate partition %s or address %s in network description", partitionId, addr)
}

func initNodeFromSeedProxy(cmd *cobra.Command, args []string) (int, *cfg.Config, *types.GenesisDoc, error) {
	s := strings.Split(args[0], ".")
	if len(s) != 2 {
		fatalf("network must be in the form of <partition-name>.<network-name>, e.g. mainnet.bvn0")
	}
	partitionName := s[0]
	networkName := s[1]

	if flagInitNode.AllowUnhealthyPeers {
		warnf("peers must be checked to use for bootstrapping when using, --allow-unhealthy-peers will have no effect")
	}

	//go gather a more robust network description
	seedProxy, err := proxy.New(flagInitNode.SeedProxy)
	check(err)

	slr := proxy.SeedListRequest{}
	slr.Network = networkName
	slr.Partition = partitionName
	slr.Sign = true
	resp, err := seedProxy.GetSeedList(context.Background(), &slr)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("proxy returned seeding error, %v", err)
	}

	//check to make sure the signature checks out.
	b, err := resp.SeedList.MarshalBinary()
	if err != nil {
		return 0, nil, nil, fmt.Errorf("invalid seed list, %v", err)
	}

	txHash := sha256.Sum256(b)
	if !resp.Signature.Verify(nil, txHash[:]) {
		return 0, nil, nil, fmt.Errorf("invalid signature from proxy")
	}

	config := cfg.Default(networkName, resp.Type, getNodeTypeFromFlag(), partitionName)

	var lastHealthyTmPeer *rpchttp.HTTP
	var lastHealthyAccPeer *client.Client
	for _, addr := range resp.Addresses {
		//go build a list of healthy nodes
		u, err := cfg.OffsetPort(addr, int(resp.BasePort), int(cfg.PortOffsetTendermintRpc))
		if err != nil {
			return 0, nil, nil, fmt.Errorf("failed to parse url from network info %s, %v", addr, err)
		}
		//check the health of the peer
		saddr := fmt.Sprintf("tcp://%s:%s", u.Hostname(), u.Port())
		peerClient, err := rpchttp.New(saddr, saddr+"/websocket")
		if err != nil {
			return 0, nil, nil, fmt.Errorf("failed to create Tendermint client for %s, %v", u.String(), err)
		}

		// peerStatus, err := peerClient.Status(context.Background())
		// if err != nil {
		// 	warnf("ignoring peer: not healthy %s", u.String())
		// 	continue
		// }

		lastHealthyTmPeer = peerClient

		//need a healthy accumulate node to further verify the proxy
		u, err = cfg.OffsetPort(addr, int(resp.BasePort), int(cfg.PortOffsetAccumulateApi))
		if err != nil {
			return 0, nil, nil, err
		}

		lastHealthyAccPeer, err = client.New(fmt.Sprintf("http://%s:%s", u.Hostname(), u.Port()))
		if err != nil {
			return 0, nil, nil, fmt.Errorf("failed to create accumulate client for %s, %v", u.String(), err)
		}

		// //if we have a healthy node with a matching id, add it as a bootstrap peer
		// if config.P2P.BootstrapPeers != "" {
		// 	config.P2P.BootstrapPeers += ","
		// }
		// u, err = cfg.OffsetPort(addr, int(resp.BasePort), int(cfg.PortOffsetTendermintP2P))
		// if err != nil {
		// 	return 0, nil, nil, err
		// }
		// config.P2P.BootstrapPeers += peerStatus.NodeInfo.NodeID.AddressString(u.String())
	}

	if lastHealthyAccPeer == nil || lastHealthyTmPeer == nil {
		return 0, nil, nil, fmt.Errorf("no healthy peers, cannot continue")
	}

	genDoc, err := getGenesis(args[0], lastHealthyTmPeer)
	if err != nil {
		return 0, nil, nil, err
	}

	//now check the registry keybook to make sure the proxy is a registered proxy
	kp := new(protocol.KeyPage)
	res := new(api.ChainQueryResponse)
	res.Data = kp
	req := new(api.GeneralQuery)
	req.Url = protocol.AccountUrl("accuproxy.acme").JoinPath("registry", "1")
	err = lastHealthyAccPeer.RequestAPIv2(context.Background(), "query", req, res)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("cannot query the accuproxy registry, %v", err)
	}

	_, _, found := kp.EntryByKeyHash(resp.Signature.GetPublicKeyHash())
	if !found {
		return 0, nil, nil, fmt.Errorf("seed proxy is not registered")
	}

	//now query the whole network configuration from the proxy.
	ncr := proxy.NetworkConfigRequest{}
	ncr.Sign = true
	ncr.Network = networkName
	nc, err := seedProxy.GetNetworkConfig(context.Background(), &ncr)
	if err != nil {
		return 0, nil, nil, err
	}

	d, err := nc.NetworkState.MarshalBinary()
	if err != nil {
		return 0, nil, nil, err
	}

	h := sha256.Sum256(d)
	if !nc.Signature.Verify(nil, h[:]) {
		return 0, nil, nil, fmt.Errorf("cannot verify network configuration from proxy")
	}
	_, _, found = kp.EntryByKeyHash(nc.Signature.GetPublicKeyHash())
	if !found {
		return 0, nil, nil, fmt.Errorf("seed proxy is not registered")
	}

	version := &api.VersionResponse{
		Version:        nc.NetworkState.Version,
		Commit:         nc.NetworkState.Commit,
		VersionIsKnown: nc.NetworkState.VersionIsKnown,
		IsTestNet:      nc.NetworkState.IsTestNet,
	}
	err = versionCheck(version, "proxy")
	if err != nil {
		return 0, nil, nil, err
	}

	config.Accumulate.Describe = cfg.Describe{NetworkType: resp.Type, PartitionId: partitionName, LocalAddress: "", Network: nc.NetworkState.Network}

	return int(resp.BasePort), config, genDoc, nil
}

func initNodeFromPeer(cmd *cobra.Command, args []string) (int, *cfg.Config, *types.GenesisDoc, error) {
	netAddr, netPort, err := resolveAddrWithPort(args[0])
	if err != nil {
		return 0, nil, nil, fmt.Errorf("invalid peer url %v", err)
	}

	accClient, err := client.New(fmt.Sprintf("http://%s:%d", netAddr, netPort+int(cfg.PortOffsetAccumulateApi)))
	if err != nil {
		return 0, nil, nil, fmt.Errorf("failed to create API client for %s, %v", args[0], err)
	}

	saddr := fmt.Sprintf("tcp://%s:%d", netAddr, netPort+int(cfg.PortOffsetTendermintRpc))
	tmClient, err := rpchttp.New(saddr, saddr+"/websocket")
	if err != nil {
		return 0, nil, nil, fmt.Errorf("failed to create Tendermint client for %s, %v", args[0], err)
	}

	version, err := getVersion(accClient)
	if err != nil {
		return 0, nil, nil, err
	}

	err = versionCheck(version, args[0])
	if err != nil {
		return 0, nil, nil, err
	}

	description, err := accClient.Describe(context.Background())
	if err != nil {
		return 0, nil, nil, fmt.Errorf("failed to get description from %s, %v", args[0], err)
	}

	if description.NetworkType == protocol.PartitionTypeBlockValidator {
		netPort -= config.PortOffsetBlockValidator
	}

	genDoc, err := getGenesis(args[0], tmClient)
	if err != nil {
		return 0, nil, nil, err
	}

	status, err := tmClient.Status(context.Background())
	if err != nil {
		return 0, nil, nil, fmt.Errorf("failed to get status of %s, %v", args[0], err)
	}

	config := cfg.Default(description.Network.Id, description.NetworkType, getNodeTypeFromFlag(), description.PartitionId)
	config.P2P.PersistentPeers = fmt.Sprintf("%s@%s:%d", status.NodeInfo.DefaultNodeID, netAddr, netPort+int(cfg.PortOffsetTendermintP2P))

	// //otherwise make the best out of what we have to establish our bootstrap peers
	// netInfo, err := tmClient.NetInfo(context.Background())
	// checkf(err, "failed to get network info from node")

	// for _, peer := range netInfo.Peers {
	// 	u, err := url.Parse(peer.RemoteIP)
	// 	checkf(err, "failed to parse url from network info %s", peer.RemoteIP)

	// 	port, err := strconv.ParseInt(u.Port(), 10, 64)
	// 	checkf(err, "failed to parse port for peer: %q", u.Port())

	// 	clientUrl := fmt.Sprintf("tcp://%s:%d", u.Hostname(), port+int64(cfg.PortOffsetTendermintRpc))

	// 	if !flagInitNode.AllowUnhealthyPeers {
	// 		//check the health of the peer
	// 		peerClient, err := rpchttp.New(clientUrl, clientUrl+"/websocket")
	// 		checkf(err, "failed to create Tendermint client for %s", u.String())

	// 		peerStatus, err := peerClient.Status(context.Background())
	// 		if err != nil {
	// 			warnf("ignoring peer: not healthy %s", clientUrl)
	// 			continue
	// 		}

	// 		if peerStatus.NodeInfo.DefaultNodeID != peer.NodeInfo.ID() {
	// 			warnf("ignoring stale peer %s", u.String())
	// 			continue
	// 		}
	// 	}

	// 	//if we have a healthy node with a matching id, add it as a bootstrap peer
	// 	config.P2P.BootstrapPeers += "," + u.String()
	// }

	config.Accumulate.Describe = cfg.Describe{
		NetworkType: description.NetworkType, PartitionId: description.PartitionId,
		LocalAddress: "", Network: description.Network}
	return netPort, config, genDoc, nil
}

func getGenesis(server string, tmClient *rpchttp.HTTP) (*types.GenesisDoc, error) {
	if flagInitNode.GenesisDoc != "" {
		genDoc, err := types.GenesisDocFromFile(flagInitNode.GenesisDoc)
		if err != nil {
			return nil, fmt.Errorf("failed to load genesis doc %q, %v", flagInitNode.GenesisDoc, err)
		}
		return genDoc, nil
	}

	if tmClient == nil {
		return nil, fmt.Errorf("no healthy tendermint peers, cannot fetch genesis document")
	}
	warnf("You are fetching the Genesis document from %s! Only do this if you trust %[1]s and your connection to it!", server)

	buf := new(bytes.Buffer)
	var total int
	for i := uint(0); ; i++ {
		if total == 0 {
			fmt.Printf("Get genesis chunk %d/? from %s\n", i+1, server)
		} else {
			fmt.Printf("Get genesis chunk %d/%d from %s\n", i+1, total, server)
		}
		rgen, err := tmClient.GenesisChunked(context.Background(), i)
		if err != nil {
			return nil, fmt.Errorf("failed to get genesis chunk %d from %s, %v", i, server, err)
		}
		total = rgen.TotalChunks
		b, err := base64.StdEncoding.DecodeString(rgen.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to decode genesis chunk %d from %s, %v", i, server, err)
		}
		_, _ = buf.Write(b)
		if i+1 >= uint(rgen.TotalChunks) {
			break
		}
	}

	doc, err := types.GenesisDocFromJSON(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to decode genesis from %s, %v", server, err)
	}

	return doc, nil
}

func initNode(cmd *cobra.Command, args []string) (string, error) {
	if !cmd.Flag("work-dir").Changed {
		return cmd.UsageString(), fmt.Errorf("Error: --work-dir flag is required\n\n")
	}

	var config *cfg.Config
	var genDoc *types.GenesisDoc
	var basePort int
	var err error
	if flagInitNode.SeedProxy != "" {
		basePort, config, genDoc, err = initNodeFromSeedProxy(cmd, args)
		if err != nil {
			return "", fmt.Errorf("failed to configure node from seed proxy, %v", err)
		}
	} else {
		basePort, config, genDoc, err = initNodeFromPeer(cmd, args)
		if err != nil {
			return "", fmt.Errorf("failed to configure node from peer, %v", err)
		}
	}

	listenUrl, err := url.Parse(fmt.Sprintf("tcp://0.0.0.0:%d", basePort))
	if err != nil {
		return "", fmt.Errorf("invalid default listen url, %v", err)
	}
	if flagInitNode.ListenIP != "" {
		listenUrl, err = url.Parse(flagInitNode.ListenIP)
		if err != nil {
			return "", fmt.Errorf("invalid --listen %q %v", flagInitNode.ListenIP, err)
		}

		if listenUrl.Port() != "" {
			p, err := strconv.ParseInt(listenUrl.Port(), 10, 16)
			if err != nil {
				return "", fmt.Errorf("invalid port number %q, %v", listenUrl.Port(), err)
			}
			basePort = int(p)
		}
	}

	var publicAddr string
	if flagInitNode.PublicIP != "" {
		publicAddr, err = resolveIp(flagInitNode.PublicIP)
		if err != nil {
			return "", fmt.Errorf("invalid public address %v", err)
		}

		partition, node, err := findInDescribe(publicAddr, config.Accumulate.PartitionId, &config.Accumulate.Network)
		if err != nil {
			return "", fmt.Errorf("cannot resolve public address in description, %v", err)
		}

		//the address wasn't found in the network description, so add it
		var localAddr string
		var port int
		if node == nil {
			u, err := ensureNodeOnPartition(partition, publicAddr, getNodeTypeFromFlag())
			if err != nil {
				return "", err
			}

			localAddr, port, err = resolveAddrWithPort(u.String())
			if err != nil {
				return "", fmt.Errorf("invalid node address %v", err)
			}
		} else {
			localAddr, port, err = resolveAddrWithPort(node.Address)
			if err != nil {
				return "", fmt.Errorf("invalid node address %v", err)
			}
		}
		//local address expect ip:port only with no scheme for connection manager to work
		config.Accumulate.LocalAddress = fmt.Sprintf("%s:%d", localAddr, port)
		config.P2P.ExternalAddress = config.Accumulate.LocalAddress
	}

	config.Accumulate.AnalysisLog.Enabled = flagInit.EnableTimingLogs
	config.Instrumentation.Prometheus = !flagInitNode.NoPrometheus

	if flagInit.LogLevels != "" {
		_, _, err := logging.ParseLogLevel(flagInit.LogLevels, io.Discard)
		if err != nil {
			return "", fmt.Errorf("--log-level, %v", err)
		}

		config.LogLevel = flagInit.LogLevels
	}

	if flagInit.Reset {
		networkReset()
	}

	netDir := netDir(config.Accumulate.Describe.NetworkType)
	config.SetRoot(filepath.Join(flagMain.WorkDir, netDir))
	accumulated.ConfigureNodePorts(&accumulated.NodeInit{
		AdvertizeAddress: listenUrl.Hostname(),
		ListenAddress:    listenUrl.Hostname(),
		BasePort:         uint64(basePort),
	}, config, config.Accumulate.Describe.NetworkType)

	config.PrivValidatorKey = "../priv_validator_key.json"

	privValKey, err := accumulated.LoadOrGenerateTmPrivKey(config.PrivValidatorKeyFile())
	DidError = err
	if err != nil {
		return "", fmt.Errorf("load/generate private key files, %v", err)
	}

	nodeKey, err := accumulated.LoadOrGenerateTmPrivKey(config.NodeKeyFile())
	if err != nil {
		return "", fmt.Errorf("load/generate node key files, %v", err)
	}

	config.Genesis = "config/genesis.snap"
	genDocBytes, err := genesis.ConvertJsonToSnapshot(genDoc)
	if err != nil {
		return "", fmt.Errorf("write node files, %v", err)
	}
	err = accumulated.WriteNodeFiles(config, privValKey, nodeKey, genDocBytes)
	if err != nil {
		return "", fmt.Errorf("write node files, %v", err)
	}

	return "", nil
}

func netDir(networkType protocol.PartitionType) string {
	switch networkType {
	case protocol.PartitionTypeDirectory:
		return "dnn"
	case protocol.PartitionTypeBlockValidator:
		return "bvnn"
	case protocol.PartitionTypeBlockSummary:
		return "bsnn"
	}
	fatalf("Unsupported network type %v", networkType)
	return ""
}

func newLogger() log.Logger {
	levels := cfg.DefaultLogLevels
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

func getNodeTypeFromFlag() cfg.NodeType {
	nodeType := cfg.Validator
	if flagInitNode.Follower || flagInitDualNode.Follower {
		nodeType = cfg.Follower
	}
	return nodeType
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
