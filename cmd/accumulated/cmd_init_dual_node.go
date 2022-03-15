package main

import (
	"fmt"
	"github.com/spf13/cobra"
	cfg "gitlab.com/accumulatenetwork/accumulate/config"
	"net"
	"path"
	"strconv"
)

func initDualNode(cmd *cobra.Command, args []string) {
	netAddr := args[0]
	addr, err := net.LookupIP(netAddr)
	checkf(err, "unknown host %s", netAddr)
	netAddr = fmt.Sprintf("%s", addr[0])

	dnBasePort, err := strconv.ParseUint(args[1], 10, 16)
	checkf(err, "invalid DN port number")

	bvnBasePort, err := strconv.ParseUint(args[2], 10, 16)
	checkf(err, "invalid BVN port number")

	workDir := flagMain.WorkDir

	flagInitNode.ListenIP = fmt.Sprintf("http://0.0.0.0:%d", dnBasePort)
	flagInitNode.SkipVersionCheck = flagInitNodeFromSeed.SkipVersionCheck
	flagInitNode.GenesisDoc = flagInitNodeFromSeed.GenesisDoc
	flagInitNode.Follower = false

	// configure the BVN first so we know how to setup the bvn.
	flagMain.WorkDir = path.Join(workDir, "bvn")
	flagInitNode.ListenIP = fmt.Sprintf("http://0.0.0.0:%d", bvnBasePort)
	args = []string{fmt.Sprintf("http://%s:%d", netAddr, bvnBasePort)}
	flagInit.Net = args[0]
	initNode(cmd, args)
	c, err := cfg.Load(path.Join(flagMain.WorkDir, "Node0"))
	fmt.Printf("%v", c.Accumulate.Network.Subnets)

	//make sure we have a block validator type
	if c.Accumulate.Network.Type != cfg.BlockValidator {
		check(fmt.Errorf("expecting block validator but received %v", c.Accumulate.Network.Type))
	}
	//now find out what bvn we are on then let
	bvnSubNet := c.Accumulate.Network.LocalSubnetID
	c.Accumulate.Network.LocalAddress

	flagMain.WorkDir = path.Join(workDir, "dn")
	args = []string{fmt.Sprintf("http://%s:%d", netAddr, dnBasePort)}
	initNode(cmd, args)

	//
	//p2pLocalAddress := fmt.Sprintf("tcp://0.0.0.0:%d", netPort+networks.TmP2pPortOffset)
	//listenIp := fmt.Sprintf("http://0.0.0.0:%d", netPort+networks.AccRouterJsonPortOffset)
	//u, err := url.Parse(listenIp)
	//checkf(err, "invalid --listen %q", listenIp)
	//
	//accClient, err := client.New(fmt.Sprintf("http://%s:%d", netAddr, netPort+networks.AccRouterJsonPortOffset))
	//checkf(err, "failed to create API client for %s", args[0])
	//
	//tmClient, err := rpchttp.New(fmt.Sprintf("tcp://%s:%d", netAddr, netPort+networks.TmRpcPortOffset))
	//checkf(err, "failed to create Tendermint client for %s", args[0])
	//
	//version := getVersion(accClient)
	//switch {
	//case !accumulate.IsVersionKnown() && !version.VersionIsKnown:
	//	warnf("The version of this executable and %s is unknown. If there is a version mismatch, the node may fail.", args[0])
	//
	//case accumulate.Commit != version.Commit:
	//	if flagInitNodeFromSeed.SkipVersionCheck {
	//		warnf("This executable is version %s but %s is %s. This may cause the node to fail.", formatVersion(accumulate.Version, accumulate.IsVersionKnown()), args[0], formatVersion(version.Version, version.VersionIsKnown))
	//	} else {
	//		fatalf("wrong version: network is %s, we are %s", formatVersion(version.Version, version.VersionIsKnown), formatVersion(accumulate.Version, accumulate.IsVersionKnown()))
	//	}
	//}
	//
	//description, err := accClient.Describe(context.Background())
	//checkf(err, "failed to get description from %s", args[0])
	//
	//var genDoc *types.GenesisDoc
	//if cmd.Flag("genesis-doc").Changed {
	//	genDoc, err = types.GenesisDocFromFile(flagInitNode.GenesisDoc)
	//	checkf(err, "failed to load genesis doc %q", flagInitNode.GenesisDoc)
	//} else {
	//	warnf("You are fetching the Genesis document from %s! Only do this if you trust %[1]s and your connection to it!", args[0])
	//	rgen, err := tmClient.Genesis(context.Background())
	//	checkf(err, "failed to get genesis from %s", args[0])
	//	genDoc = rgen.Genesis
	//}
	//
	//status, err := tmClient.Status(context.Background())
	//checkf(err, "failed to get status of %s", args[0])
	//
	//nodeType := cfg.Validator
	//if flagInitNode.Follower {
	//	nodeType = cfg.Follower
	//}
	//config := config.Default(description.Subnet.Type, nodeType, description.Subnet.LocalSubnetID)
	////config.P2P.PersistentPeers = fmt.Sprintf("%s@%s:%d", status.NodeInfo.NodeID, netAddr, netPort+networks.TmP2pPortOffset)
	////the bootstrap needs to be a sentry node.
	//config.P2P.BootstrapPeers = fmt.Sprintf("%s@%s:%d", status.NodeInfo.NodeID, netAddr, netPort+networks.TmP2pPortOffset)
	//config.Accumulate.Network = description.Subnet
	//
	////if flagInit.Net != "" {
	//config.Accumulate.Network.LocalAddress = parseHost(p2pLocalAddress)
	////need to find the subnet and add the local address to it as well.
	////prepend the local node to the local list.
	//localNode := cfg.Node{Address: p2pLocalAddress, Type: nodeType}
	//for i, s := range config.Accumulate.Network.Subnets {
	//	var validNodes []cfg.Node
	//	//if s.ID == config.Accumulate.Network.LocalSubnetID {
	//	//loop through all the nodes and add persistent peers
	//	for j, n := range s.Nodes {
	//		//don't bother to fetch if we already have it.
	//		nodeHost, _, err := net.SplitHostPort(parseHost(n.Address))
	//		if err != nil {
	//			warnf("invalid host from node %s", n.Address)
	//			continue
	//		}
	//		if netAddr != nodeHost {
	//			tmClient, err := rpchttp.New(fmt.Sprintf("tcp://%s:%d", nodeHost, netPort+networks.TmRpcPortOffset))
	//			if err != nil {
	//				warnf("failed to create Tendermint client for %s with error %v", n.Address, err)
	//				continue
	//			}
	//
	//			status, err := tmClient.Status(context.Background())
	//			if err != nil {
	//				warnf("failed to get status of %s with error %v", n.Address, err)
	//				continue
	//			}
	//
	//			peers := config.P2P.BootstrapPeers
	//			config.P2P.BootstrapPeers = fmt.Sprintf("%s,%s@%s:%d", peers,
	//				status.NodeInfo.NodeID, nodeHost, netPort+networks.TmP2pPortOffset)
	//			validNodes = append(validNodes, s.Nodes[j])
	//		}
	//	}
	//	//}
	//	//prepend the new node to the list
	//	config.Accumulate.Network.Subnets[i].Nodes = append([]cfg.Node{localNode}, validNodes...)
	//	break
	//}
	////}
	//
	//if flagInit.LogLevels != "" {
	//	_, _, err := logging.ParseLogLevel(flagInit.LogLevels, io.Discard)
	//	checkf(err, "--log-level")
	//	config.LogLevel = flagInit.LogLevels
	//}
	//
	//if len(flagInit.Etcd) > 0 {
	//	config.Accumulate.Storage.Type = cfg.EtcdStorage
	//	config.Accumulate.Storage.Etcd = new(etcd.Config)
	//	config.Accumulate.Storage.Etcd.Endpoints = flagInit.Etcd
	//	config.Accumulate.Storage.Etcd.DialTimeout = 5 * time.Second
	//}
	//
	//if flagInit.Reset {
	//	nodeReset()
	//}
	//
	//check(node.Init(node.InitOptions{
	//	WorkDir:    flagMain.WorkDir,
	//	Port:       netPort + networks.AccRouterJsonPortOffset,
	//	GenesisDoc: genDoc,
	//	Config:     []*cfg.Config{config},
	//	RemoteIP:   []string{u.Hostname()},
	//	ListenIP:   []string{u.Hostname()},
	//	Logger:     newLogger(),
	//}))
}
