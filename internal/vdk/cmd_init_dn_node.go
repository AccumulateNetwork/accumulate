package vdk

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

var cmdInitDNNode = &cobra.Command{
	Use:   "dn <[partition.network] | [peer dn url]>",
	Short: "Initialize a dn node using the either a dn url as a peer, or by specifying the partition.network name and --seed https://seedproxy",
	Run:   initDNNode,
	Args:  cobra.ExactArgs(1),
}

func setFlagsForInit() error {
	var err error
	if flagInitDNNode.PublicIP == "" {
		flagInitNode.PublicIP, err = resolvePublicIp()
		if err != nil {
			return fmt.Errorf("cannot resolve public ip address, %v", err)
		}
	} else {
		flagInitNode.PublicIP = flagInitDNNode.PublicIP
	}

	flagInitNode.SkipVersionCheck = flagInitDNNode.SkipVersionCheck
	flagInitNode.GenesisDoc = flagInitDNNode.GenesisDoc
	flagInitNode.SeedProxy = flagInitDNNode.SeedProxy
	flagInitNode.Follower = true
	flagInitNode.NoPrometheus = flagInitDNNode.NoPrometheus
	if flagInitDNNode.ListenIP != "" {
		listenUrl, err := url.Parse(flagInitDNNode.ListenIP)
		if err != nil {
			return fmt.Errorf("invalid --listen %q %v", flagInitDNNode.ListenIP, err)
		}
		flagInitNode.ListenIP = "tcp://" + listenUrl.Hostname()
	}
	return nil
}

func initDNNodeFromSeed(cmd *cobra.Command, args []string) error {
	s := strings.Split(args[0], ".")
	if len(s) != 2 {
		fatalf("network must be in the form of <network-name>.<partition-name>, e.g. mainnet.dn")
	}
	partitionName := s[0]
	networkName := s[1]
	if partitionName == "Directory" {
		return fmt.Errorf("cannot specify \"Directory\" partition, please specify a block validator name for init dn node")
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

	return nil
}

// func initDualNodeFromPeer(cmd *cobra.Command, args []string) error {
// 	u, err := url.Parse(args[0])
// 	check(err)

// 	host := u.Hostname()
// 	port := u.Port()
// 	if port == "" {
// 		fatalf("cannot resolve host and port %v", args[0])
// 	}
// 	bvnHost := u.String()

// 	_, err = net.LookupIP(host)
// 	checkf(err, "unknown host %s", u.Hostname())

// 	bvnBasePort, err := strconv.ParseUint(port, 10, 16)
// 	checkf(err, "invalid DN port number")
// 	dnBasePort := bvnBasePort - uint64(cfg.PortOffsetBlockValidator)

// 	err = setFlagsForInit()
// 	if err != nil {
// 		return err
// 	}

// 	// configure the directory node
// 	dnnUrl := fmt.Sprintf("%s://%s:%d", u.Scheme, u.Hostname(), dnBasePort)
// 	args = []string{dnnUrl}

// 	_, err = initNode(cmd, args)
// 	if err != nil {
// 		return err
// 	}

// 	//finalize BVNN
// 	c, err := finalizeBvnn()
// 	if err != nil {
// 		return err
// 	}

// 	_, err = finalizeDnn(c.Accumulate.PartitionId)
// 	if err != nil {
// 		return fmt.Errorf("error finalizing dnn configuration, %v", err)
// 	}

// 	return nil
// }

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

// initDNNode accumulate `init dn http://ip:bvnport` or `init dn partition.network --seed https://seednode
func initDNNode(cmd *cobra.Command, args []string) {
	var err error
	if flagInitDualNode.SeedProxy != "" {
		err = initDualNodeFromSeed(cmd, args)
	} else {
		err = initDualNodeFromPeer(cmd, args)
	}
	check(err)
}
