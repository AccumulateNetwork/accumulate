// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	cfg "gitlab.com/accumulatenetwork/accumulate/config"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdInitDualNode = &cobra.Command{
	Use:   "dual <[partition.network] | [peer bvn url]>",
	Short: "Initialize a dual node using the either a bvn url as a peer, or by specifying the partition.network name and --seed https://seedproxy",
	Run:   initDualNode,
	Args:  cobra.ExactArgs(1),
}

func setFlagsForInit() error {
	var err error
	if flagInitDualNode.PublicIP == "" {
		flagInitNode.PublicIP, err = resolvePublicIp()
		if err != nil {
			return fmt.Errorf("cannot resolve public ip address, %v", err)
		}
	} else {
		flagInitNode.PublicIP = flagInitDualNode.PublicIP
	}

	flagInitNode.SkipVersionCheck = flagInitDualNode.SkipVersionCheck
	flagInitNode.GenesisDoc = flagInitDualNode.GenesisDoc
	flagInitNode.SeedProxy = flagInitDualNode.SeedProxy
	flagInitNode.Follower = false
	flagInitNode.NoPrometheus = flagInitDualNode.NoPrometheus
	if flagInitDualNode.ListenIP != "" {
		listenUrl, err := url.Parse(flagInitDualNode.ListenIP)
		if err != nil {
			return fmt.Errorf("invalid --listen %q %v", flagInitDualNode.ListenIP, err)
		}
		flagInitNode.ListenIP = "tcp://" + listenUrl.Hostname()
	}
	return nil
}

func initDualNodeFromSeed(cmd *cobra.Command, args []string) error {
	s := strings.Split(args[0], ".")
	if len(s) != 2 {
		fatalf("network must be in the form of <network-name>.<partition-name>, e.g. mainnet.bvn0")
	}
	partitionName := s[0]
	networkName := s[1]
	if partitionName == "Directory" {
		return fmt.Errorf("cannot specify \"Directory\" partition, please specify a block validator name for init dual node")
	}
	_ = networkName

	err := setFlagsForInit()
	if err != nil {
		return err
	}

	// configure the Directory first so we know how to setup the bvn.
	args = []string{args[0]}

	_, err = initNode(cmd, args)
	if err != nil {
		return fmt.Errorf("cannot configure the directory node, %v", err)
	}

	c, err := finalizeDnn(partitionName)
	if err != nil {
		return err
	}

	partition, _, err := findInDescribe("", partitionName, &c.Accumulate.Network)
	if err != nil {
		return fmt.Errorf("cannot find partition %s in network configuration, %v", partitionName, err)
	}

	if partition.Type == cfg.NetworkTypeDirectory {
		return fmt.Errorf("network partition of second node configuration must be a block validator. Please specify {network-name}.{bvn-partition-id} first parameter to init dual")
	}

	bvnHost, err := findHealthyNodeOnPartition(partition)
	if err != nil {
		return fmt.Errorf("cannot find a healthy node on partition %s, %v", partitionName, err)
	}

	args = []string{fmt.Sprintf("tcp://%s:%d", bvnHost, partition.BasePort)}

	_, err = initNode(cmd, args)
	if err != nil {
		return fmt.Errorf("cannot configure the directory node, %v", err)
	}

	_, err = finalizeBvnn()
	return err
}

func initDualNodeFromPeer(cmd *cobra.Command, args []string) error {
	u, err := url.Parse(args[0])
	check(err)

	host := u.Hostname()
	port := u.Port()
	if port == "" {
		fatalf("cannot resolve host and port %v", args[0])
	}
	bvnHost := u.String()

	_, err = net.LookupIP(host)
	checkf(err, "unknown host %s", u.Hostname())

	bvnBasePort, err := strconv.ParseUint(port, 10, 16)
	checkf(err, "invalid DN port number")
	dnBasePort := bvnBasePort - uint64(cfg.PortOffsetBlockValidator)

	err = setFlagsForInit()
	if err != nil {
		return err
	}

	// configure the directory node
	dnnUrl := fmt.Sprintf("%s://%s:%d", u.Scheme, u.Hostname(), dnBasePort)
	args = []string{dnnUrl}

	_, err = initNode(cmd, args)
	if err != nil {
		return err
	}

	args = []string{bvnHost}

	_, err = initNode(cmd, args)
	if err != nil {
		return err
	}

	//finalize BVNN
	c, err := finalizeBvnn()
	if err != nil {
		return err
	}

	_, err = finalizeDnn(c.Accumulate.PartitionId)
	if err != nil {
		return fmt.Errorf("error finalizing dnn configuration, %v", err)
	}

	return nil
}

func finalizeDnn(bvnId string) (*cfg.Config, error) {
	c, err := cfg.Load(filepath.Join(flagMain.WorkDir, "dnn"))
	if err != nil {
		return nil, err
	}

	//make sure we have a block validator type
	if c.Accumulate.NetworkType != cfg.Directory {
		return nil, fmt.Errorf("expecting directory but received %v", c.Accumulate.NetworkType)
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

	bvn := c.Accumulate.Network.GetPartitionByID(bvnId)
	if bvn == nil {
		return nil, fmt.Errorf("bvn partition not found in configuration, %s", bvnId)
	}

	_, err = ensureNodeOnPartition(bvn, c.Accumulate.LocalAddress, cfg.NodeTypeValidator)
	if err != nil {
		return nil, err
	}

	err = cfg.Store(c)
	if err != nil {
		return nil, fmt.Errorf("cannot store configuration file for node, %v", err)
	}

	return c, nil
}

func finalizeBvnn() (*cfg.Config, error) {
	c, err := cfg.Load(filepath.Join(flagMain.WorkDir, "bvnn"))
	if err != nil {
		return nil, fmt.Errorf("cannot load configuration file for node, %v", err)
	}

	if c.Accumulate.NetworkType != cfg.NetworkTypeBlockValidator {
		return nil, fmt.Errorf("network partition of second node configuration must be a block validator. Please specify {network-name}.{bvn-partition-id} first parameter to init dual")
	}

	if flagInit.NoEmptyBlocks {
		c.Consensus.CreateEmptyBlocks = false
	}
	if flagInit.NoWebsite {
		c.Accumulate.Website.Enabled = false
	}

	//in dual mode, the key between bvn and dn is shared.
	//This will be cleaned up when init system is overhauled with AC-1263
	if len(c.P2P.PersistentPeers) > 0 {
		c.P2P.BootstrapPeers = c.P2P.PersistentPeers
		c.P2P.PersistentPeers = ""
	}

	dn := c.Accumulate.Network.GetPartitionByID(protocol.Directory)
	if dn == nil {
		return nil, fmt.Errorf("cannot find directory parition on network in configuration")
	}

	_, err = ensureNodeOnPartition(dn, c.Accumulate.LocalAddress, cfg.NodeTypeValidator)
	if err != nil {
		return nil, err
	}

	return c, cfg.Store(c)
}

// initDualNode accumulate `init dual http://ip:bvnport` or `init dual partition.network --seed https://seednode
func initDualNode(cmd *cobra.Command, args []string) {
	if flagInit.Reset {
		flagInit.Reset = false
		networkReset()
	}

	var err error
	if flagInitDualNode.SeedProxy != "" {
		err = initDualNodeFromSeed(cmd, args)
	} else {
		err = initDualNodeFromPeer(cmd, args)
	}
	check(err)
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
		addr, err := resolveAddr(p.Address)
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

func ensureNodeOnPartition(partition *cfg.Partition, addr string, t cfg.NodeType) (*url.URL, error) {
	testAddr, err := resolveIp(addr)
	if err != nil {
		return nil, err
	}

	for _, n := range partition.Nodes {
		nodeAddr, err := resolveAddr(n.Address)
		if err != nil {
			return nil, err
		}

		if strings.EqualFold(nodeAddr, testAddr) {
			return url.Parse(testAddr)
		}
	}

	//set port on url for partition, we need to add it to keep the connection mgr sane
	u, err := cfg.OffsetPort(testAddr, int(partition.BasePort), int(cfg.PortOffsetTendermintP2P))
	if err != nil {
		return nil, err
	}

	partition.Nodes = append(partition.Nodes, cfg.Node{Address: u.String(), Type: t})
	return u, nil
}
