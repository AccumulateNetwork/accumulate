// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package node

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"path"
	"strings"

	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/cometbft/cometbft/types"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/proxy"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/vdk/logger"
)

// CreateNode specify working directory of configuration, a log writer callback,
// and nodeIndex (usually zero unless running a devnet with more than one node on the same BVN)
func NewNode(workDir string, logWriter logger.LogWriter, nodeIndex int) (*Daemon, error) {
	node, err := accumulated.Load(workDir, func(c *config.Config) (io.Writer, error) {
		la := func(w io.Writer, format string, color bool) io.Writer {
			config := logger.NodeWriterConfig{
				Format:          logger.NodeLogFormat(format),
				PartitionName:   c.Accumulate.PartitionId,
				NodeIndex:       nodeIndex,
				NodeName:        "node",
				NodeNamePadding: 0,
				Colorize:        color}
			return logger.NewNodeWriter(w, config)
		}
		return logWriter(c.LogFormat, la)
	})

	if err != nil {
		return nil, err //err
	}
	return node, nil
}

func InitializeFollowerFromSeed(workDir string, expectedPartitionType protocol.PartitionType, seedNodeUrl string) error {
	basePort, config, genDoc, err := initFollowerNodeFromSeedNodeUrl(seedNodeUrl)
	if err != nil {
		return fmt.Errorf("failed to configure node from seed proxy, %v", err)
	}
	listenUrl, err := url.Parse(fmt.Sprintf("tcp://0.0.0.0:%d", basePort))
	if err != nil {
		return fmt.Errorf("invalid default listen url, %v", err)
	}
	config.Accumulate.AnalysisLog.Enabled = true
	config.Instrumentation.Prometheus = true
	config.LogLevel = "info"

	if config.Accumulate.Describe.NetworkType != expectedPartitionType {
		return fmt.Errorf("expecting directory node partition")
	}
	netDir := netDir(config.Accumulate.Describe.NetworkType)
	config.SetRoot(path.Join(workDir, netDir))
	accumulated.ConfigureNodePorts(&accumulated.NodeInit{
		AdvertizeAddress: listenUrl.Hostname(),
		ListenAddress:    listenUrl.Hostname(),
		BasePort:         uint64(basePort),
	}, config, config.Accumulate.Describe.NetworkType)

	config.PrivValidatorKey = "../priv_validator_key.json"

	privValKey, err := accumulated.LoadOrGenerateTmPrivKey(config.PrivValidatorKeyFile())
	if err != nil {
		return fmt.Errorf("load/generate private key files, %v", err)
	}

	nodeKey, err := accumulated.LoadOrGenerateTmPrivKey(config.NodeKeyFile())
	if err != nil {
		return fmt.Errorf("load/generate node key files, %v", err)
	}

	config.Genesis = "config/genesis.snap"
	genDocBytes, err := genesis.ConvertJsonToSnapshot(genDoc)
	if err != nil {
		return fmt.Errorf("write node files, %v", err)
	}
	err = accumulated.WriteNodeFiles(config, privValKey, nodeKey, genDocBytes)
	if err != nil {
		return fmt.Errorf("write node files, %v", err)
	}
	return nil
}

func initFollowerNodeFromSeedNodeUrl(seedNodeUrl string) (int, *config.Config, *types.GenesisDoc, error) {
	s := strings.Split(seedNodeUrl, ".")
	if len(s) != 2 {
		return 0, nil, nil, fmt.Errorf("network must be in the form of <partition-name>.<network-name>, e.g. mainnet.bvn0")
	}
	partitionName := s[0]
	networkName := s[1]

	//go gather a more robust network description
	seedProxy, err := proxy.New(seedNodeUrl)
	if err != nil {
		return 0, nil, nil, err
	}

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
	if !resp.Signature.Verify(nil, txHash[:], nil) {
		return 0, nil, nil, fmt.Errorf("invalid signature from proxy")
	}

	cfg := config.Default(networkName, resp.Type, config.Follower, partitionName)

	var lastHealthyTmPeer *rpchttp.HTTP
	var lastHealthyAccPeer *client.Client
	for _, addr := range resp.Addresses {
		//go build a list of healthy nodes
		u, err := config.OffsetPort(addr, int(resp.BasePort), int(config.PortOffsetTendermintRpc))
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
		u, err = config.OffsetPort(addr, int(resp.BasePort), int(config.PortOffsetAccumulateApi))
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

	genDoc, err := getGenesis(seedNodeUrl, lastHealthyTmPeer)
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
	if !nc.Signature.Verify(nil, h[:], nil) {
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

	// config.Accumulate.Describe = config.Describe{NetworkType: resp.Type, PartitionId: partitionName, LocalAddress: "", Network: nc.NetworkState.Network}

	return int(resp.BasePort), cfg, genDoc, nil
}

func getGenesis(server string, tmClient *rpchttp.HTTP) (*types.GenesisDoc, error) {
	if tmClient == nil {
		return nil, fmt.Errorf("no healthy tendermint peers, cannot fetch genesis document")
	}
	log.Printf("You are fetching the Genesis document from %s! Only do this if you trust %[1]s and your connection to it!", server)

	buf := new(bytes.Buffer)
	var total int
	for i := uint(0); ; i++ {
		if total == 0 {
			log.Printf("Get genesis chunk %d/? from %s\n", i+1, server)
		} else {
			log.Printf("Get genesis chunk %d/%d from %s\n", i+1, total, server)
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

func versionCheck(version *api.VersionResponse, peer string) error {
	switch {
	case !accumulate.IsVersionKnown() && !version.VersionIsKnown:
		log.Printf("The version of this executable and %s is unknown. If there is a version mismatch, the node may fail.\n", peer)

	case accumulate.Commit != version.Commit:
		return fmt.Errorf("wrong version: network is %s, we are %s", formatVersion(version.Version, version.VersionIsKnown), formatVersion(accumulate.Version, accumulate.IsVersionKnown()))
	}
	return nil
}

func formatVersion(version string, known bool) string {
	if !known {
		return "unknown"
	}
	return version
}

func netDir(networkType protocol.PartitionType) string {

	switch networkType {
	case protocol.PartitionTypeDirectory:
		return "dnn"
	case protocol.PartitionTypeBlockValidator:
		return "bvnn"
	}
	log.Fatalf("Unsupported network type %v", networkType)
	return ""
}
