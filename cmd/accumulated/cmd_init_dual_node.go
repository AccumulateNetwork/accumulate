package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	cfg "gitlab.com/accumulatenetwork/accumulate/config"
)

var cmdInitDualNode = &cobra.Command{
	Use:   "dual <url|ip> <dn base port> <bvn base port>",
	Short: "Initialize a dual run from seed IP, DN base port, and BVN base port",
	Run:   initDualNode,
	Args:  cobra.ExactArgs(2),
}

// initDualNode accumulate init dual Mainnet.BVN0 http://ip:dnport
func initDualNode(cmd *cobra.Command, args []string) {
	s := strings.Split(args[0], ".")
	if len(s) != 2 {
		fatalf("network must be in the form of <network-name>.<partition-name>, e.g. mainnet.bvn0")
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

	flagInitNode.ListenIP = fmt.Sprintf("http://0.0.0.0:%d", dnBasePort)
	if flagInitDualNode.PublicIP == "" {
		flagInitNode.PublicIP, err = resolvePublicIp()
		checkf(err, "cannot resolve public ip address")
	} else {
		flagInitNode.PublicIP = flagInitDualNode.PublicIP
	}
	flagInitNode.SkipVersionCheck = flagInitDualNode.SkipVersionCheck
	flagInitNode.GenesisDoc = flagInitDualNode.GenesisDoc
	flagInitNode.SeedProxy = flagInitDualNode.SeedProxy
	flagInitNode.Follower = false

	// configure the BVN first so we know how to setup the bvn.
	args = []string{u.String()}

	initNode(cmd, args)

	c, err := cfg.Load(path.Join(flagMain.WorkDir, "dnn"))
	check(err)

	//make sure we have a block validator type
	if c.Accumulate.NetworkType != cfg.Directory {
		fatalf("expecting directory but received %v", c.Accumulate.NetworkType)
	}

	//now find out what bvn we are on then let
	_ = netAddr

	bvn, _, err := findInDescribe("", partitionName, &c.Accumulate.Network)
	checkf(err, "partition %s not found in bvn configuration", partitionName)

	if len(bvn.Nodes) == 0 {
		fatalf("no bvn nodes defined in network partition %s", partitionName)
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

	flagInitNode.ListenIP = fmt.Sprintf("http://0.0.0.0:%v", bvn.BasePort)
	args = []string{bvn.Nodes[0].Address}
	fmt.Println(dnWebHostUrl)
	initNode(cmd, args)

	c, err = cfg.Load(path.Join(flagMain.WorkDir, "bvnn"))

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

	if len(c.P2P.PersistentPeers) > 0 {
		c.P2P.BootstrapPeers = c.P2P.PersistentPeers
		c.P2P.PersistentPeers = ""
	}

	err = cfg.Store(c)
	checkf(err, "cannot store configuration file for node")
}

func resolvePublicIp() (string, error) {
	req, err := http.Get("http://ip-api.com/json/")
	if err != nil {
		return "", err
	}
	defer req.Body.Close()

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return "", err
	}

	ip := struct {
		Query string
	}{}
	json.Unmarshal(body, &ip)
	return ip.Query, nil
}
