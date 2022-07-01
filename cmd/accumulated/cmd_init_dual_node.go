package main

import (
	"context"
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
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	cfg "gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
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

	flagInitNode.ListenIP = fmt.Sprintf("http://0.0.0.0:%v", dnBasePort+cfg.PortOffsetBlockValidator)

	partition, _, err := findInDescribe("", partitionName, &c.Accumulate.Network)
	checkf(err, "cannot find partition %s in network configuration", partitionName)

	if partition.Type == cfg.NetworkTypeDirectory {
		fatalf("network partition of second node configuration must be a block validator. Please specify {network-name}.{bvn-partition-id} first parameter to init dual")
	}
	bvnHost, err := findHealthyNodeOnPartition(partition)
	checkf(err, "cannot find a healthy node on partition %s", partitionName)

	args = []string{fmt.Sprintf("tcp://%s:%d", bvnHost, dnBasePort+cfg.PortOffsetBlockValidator)}

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
	err = json.Unmarshal(body, &ip)
	if err != nil {
		return "", err
	}
	return ip.Query, nil
}

func findHealthyNodeOnPartition(partition *cfg.Partition) (string, error) {
	for _, p := range partition.Nodes {
		addr, _, err := resolveAddr(p.Address)
		if err != nil {
			continue
		}

		accClient, err := client.New(fmt.Sprintf("http://%s:%d", addr, partition.BasePort+int64(cfg.PortOffsetAccumulateApi)))
		if err != nil {
			continue
		}
		tmClient, err := rpchttp.New(fmt.Sprintf("tcp://%s:%d", addr, partition.BasePort+int64(cfg.PortOffsetTendermintRpc)))
		if err != nil {
			continue
		}

		_, err = accClient.Describe(context.Background())
		if err != nil {
			continue
		}

		_, err = tmClient.Status(context.Background())
		if err != nil {
			continue
		}
		//if we get here, assume we have a viable node
		return addr, nil
	}
	return "", fmt.Errorf("no viable node found on partition %s", partition.Id)
}
