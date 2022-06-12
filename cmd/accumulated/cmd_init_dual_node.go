package main

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	cfg "gitlab.com/accumulatenetwork/accumulate/config"
)

// initDualNode accumulate init dual Mainnet.BVN0 http://ip:dnport
func initDualNode(cmd *cobra.Command, args []string) {
	s := strings.Split(args[0], ".")
	if len(s) != 2 {
		fatalf("network must be in the form of <network-name>.<subnet-name>, e.g. mainnet.bvn0")
	}
	networkName := s[0]
	partitionName := s[1]
	_ = networkName

	u, err := url.Parse(args[1])
	check(err)

	host := u.Hostname()
	port := u.Port()
	if port == "" {
		fatalf("cannot resolve host and port %v", args[1])
	}

	addr, err := net.LookupIP(host)
	checkf(err, "unknown host %s", u.Hostname())
	netAddr := addr[0].String()

	dnBasePort, err := strconv.ParseUint(port, 10, 16)
	checkf(err, "invalid DN port number")

	workDir := flagMain.WorkDir

	flagInitNode.ListenIP = fmt.Sprintf("http://0.0.0.0:%d", dnBasePort)
	flagInitNode.SkipVersionCheck = flagInitDualNode.SkipVersionCheck
	flagInitNode.GenesisDoc = flagInitDualNode.GenesisDoc
	flagInitNode.SeedProxy = flagInitDualNode.SeedProxy
	flagInitNode.Follower = false

	// configure the BVN first so we know how to setup the bvn.
	flagMain.WorkDir = path.Join(workDir, "dn")
	args = []string{"0", u.String()}
	//flagInit.Net = args[0]
	initNode(cmd, args)
	dnNodePath := path.Join(flagMain.WorkDir, "Node0")
	c, err := cfg.Load(dnNodePath)
	check(err)

	//make sure we have a block validator type
	if c.Accumulate.NetworkType != cfg.Directory {
		fatalf("expecting directory but received %v", c.Accumulate.NetworkType)
	}

	//now find out what bvn we are on then let
	dnPartition := c.Accumulate.LocalAddress
	dnHost, _, err := net.SplitHostPort(dnPartition)
	checkf(err, "cannot resolve bvn host and port")

	_ = netAddr

	var bvn *cfg.Partition
	for i, v := range c.Accumulate.Network.Partitions {
		//search for the directory.
		if v.Id == partitionName {
			bvn = &c.Accumulate.Network.Partitions[i]
			break
		}
	}

	if bvn == nil {
		fatalf("directory not found in bvn configuration")
	}

	// now search for the dn associated with the local address
	var bvnHost *cfg.Node
	var bvnBasePort string
	var bvnHostIP string
	for i, v := range bvn.Nodes {
		//loop through the nodes searching for this bvn.
		u, err := url.Parse(v.Address)
		checkf(err, "cannot resolve dn host and port")
		bvnHostIP = u.Hostname()
		bvnBasePort = u.Port()
		if dnHost == bvnHostIP {
			bvnHost = &bvn.Nodes[i]
		}
	}

	if bvnHost == nil {
		fatalf("bvn host not found in %v partition", partitionName)
	}

	if flagInit.NoEmptyBlocks {
		c.Consensus.CreateEmptyBlocks = false
	}
	if flagInit.NoWebsite {
		c.Accumulate.Website.Enabled = false
	}

	if len(c.P2P.PersistentPeers) > 0 {
		c.P2P.BootstrapPeers = c.P2P.PersistentPeers
		c.P2P.PersistentPeers = ""
	}
	dnWebHostUrl, err := url.Parse(c.Accumulate.Website.ListenAddress)
	checkf(err, "cannot parse website listen address (%v) for node", c.Accumulate.Website.ListenAddress)

	err = cfg.Store(c)
	checkf(err, "cannot store configuration file for node")

	flagInitNode.ListenIP = fmt.Sprintf("http://0.0.0.0:%v", bvnBasePort)
	flagMain.WorkDir = path.Join(workDir, "bvn")
	args = []string{"0", bvnHost.Address}
	initNode(cmd, args)

	bvnNodePath := path.Join(flagMain.WorkDir, "Node0")

	c, err = cfg.Load(bvnNodePath)

	checkf(err, "cannot load configuration file for node")

	if flagInit.NoEmptyBlocks {
		c.Consensus.CreateEmptyBlocks = false
	}
	if flagInit.NoWebsite {
		c.Accumulate.Website.Enabled = false
	}
	webPort, err := strconv.ParseUint(dnWebHostUrl.Port(), 10, 16)
	checkf(err, "invalid port for bvn website (%v)", dnWebHostUrl.Port())
	c.Accumulate.Website.ListenAddress = fmt.Sprintf("http://%s:%d", dnWebHostUrl.Hostname(), webPort+1)

	//in dual mode, the key between bvn and dn is shared.
	//This will be cleaned up when init system is overhauled with AC-1263
	if c.PrivValidator != nil {
		c.PrivValidator.Key = path.Join(dnNodePath, "/config/priv_validator_key.json")
	}
	os.Remove(path.Join(bvnNodePath, "/config/priv_validator_key.json"))
	if len(c.P2P.PersistentPeers) > 0 {
		c.P2P.BootstrapPeers = c.P2P.PersistentPeers
		c.P2P.PersistentPeers = ""
	}

	err = cfg.Store(c)
	checkf(err, "cannot store configuration file for node")
}
