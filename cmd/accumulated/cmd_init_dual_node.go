package main

import (
	"fmt"
	"github.com/spf13/cobra"
	cfg "gitlab.com/accumulatenetwork/accumulate/config"
	"net"
	"net/url"
	"path"
	"strconv"
)

// initDualNode accumulate init dual BVN0 http://ip:dnport
func initDualNode(cmd *cobra.Command, args []string) {

	subnetName := args[0]
	u, err := url.Parse(args[1])
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		check(fmt.Errorf("cannot resolve host and port %v", args[1]))
	}

	addr, err := net.LookupIP(host)
	checkf(err, "unknown host %s", u.Hostname())
	netAddr := fmt.Sprintf("%s", addr[0])

	dnBasePort, err := strconv.ParseUint(port, 10, 16)
	checkf(err, "invalid DN port number")

	workDir := flagMain.WorkDir

	flagInitNode.ListenIP = fmt.Sprintf("http://0.0.0.0:%d", dnBasePort)
	flagInitNode.SkipVersionCheck = flagInitNodeFromSeed.SkipVersionCheck
	flagInitNode.GenesisDoc = flagInitNodeFromSeed.GenesisDoc
	flagInitNode.Follower = false

	// configure the BVN first so we know how to setup the bvn.
	flagMain.WorkDir = path.Join(workDir, "dn")
	args = []string{u.String()}
	//flagInit.Net = args[0]
	initNode(cmd, args)
	c, err := cfg.Load(path.Join(flagMain.WorkDir, "Node0"))
	fmt.Printf("%v", c.Accumulate.Network.Subnets)

	//make sure we have a block validator type
	if c.Accumulate.Network.Type != cfg.Directory {
		check(fmt.Errorf("expecting directory but received %v", c.Accumulate.Network.Type))
	}

	//now find out what bvn we are on then let
	dnSubNet := c.Accumulate.Network.LocalAddress
	dnHost, port, err := net.SplitHostPort(dnSubNet)
	checkf(err, "cannot resolve bvn host and port")

	_ = netAddr

	var bvn *cfg.Subnet
	for i, v := range c.Accumulate.Network.Subnets {
		//search for the directory.
		if v.ID == subnetName {
			bvn = &c.Accumulate.Network.Subnets[i]
			break
		}
	}

	if bvn == nil {
		check(fmt.Errorf("directory not found in bvn configuration"))
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
		check(fmt.Errorf("bvn host not found in %v subnet", subnetName))
	}

	flagInit.NoWebsite = true
	flagInitNode.ListenIP = fmt.Sprintf("http://0.0.0.0:%v", bvnBasePort)
	flagMain.WorkDir = path.Join(workDir, "bvn")
	args = []string{bvnHost.Address}
	initNode(cmd, args)

}
