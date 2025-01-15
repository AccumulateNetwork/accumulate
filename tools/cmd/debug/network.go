// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	stdurl "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	cmtp2p "github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/rpc/client"
	"github.com/cometbft/cometbft/rpc/client/http"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cometbft/cometbft/rpc/jsonrpc/types"
	"github.com/fatih/color"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/exp/promise"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/healing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	cmdutil "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	v2 "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var networkCmd = &cobra.Command{
	Use: "network",
}

var networkScanCmd = &cobra.Command{
	Use:   "scan [network]",
	Short: "Scan the network for nodes",
	Args:  cobra.ExactArgs(1),
	Run:   scanNetwork,
}

var networkScanNodeCmd = &cobra.Command{
	Use:   "scan-node [address]",
	Short: "Scan a node",
	Args:  cobra.ExactArgs(1),
	Run:   scanNode,
}

var networkStatusCmd = &cobra.Command{
	Use:   "status [network]",
	Short: "Check the status of the network",
	Args:  cobra.ExactArgs(1),
	Run:   networkStatus,
}

type nodePartStatus struct {
	Validator bool   `json:"validator"`
	Height    uint64 `json:"height"`
	Zombie    bool   `json:"zombie"`
}

type nodeStatus struct {
	PeerID   peer.ID                    `json:"peerID"`
	Key      []byte                     `json:"key"`
	Operator *url.URL                   `json:"operator"`
	Parts    map[string]*nodePartStatus `json:"parts"`
	Host     string                     `json:"host"`
	Version  string                     `json:"version"`
	API      struct {
		ConnectV3 bool `json:"connectV3"`
		QueryV3   bool `json:"queryV3"`
	} `json:"api"`
}

func init() {
	cmd.AddCommand(networkCmd)
	networkCmd.AddCommand(
		networkScanCmd,
		networkScanNodeCmd,
		networkStatusCmd,
	)

	networkCmd.PersistentFlags().BoolVarP(&outputJSON, "json", "j", false, "Output result as JSON")
	networkStatusCmd.PersistentFlags().StringVar(&cachedScan, "cached-scan", "", "A cached network scan")
}

func scanNetwork(_ *cobra.Command, args []string) {
	client := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(args[0], "v3"))
	client.Client.Timeout = time.Hour
	net, err := healing.ScanNetwork(context.Background(), client)
	check(err)

	if outputJSON {
		check(json.NewEncoder(os.Stdout).Encode(net))
		return
	}

	for _, part := range net.Status.Network.Partitions {
		fmt.Println(part.ID)
		for _, peer := range net.Peers[part.ID] {
			fmt.Printf("  %v\n", peer)
		}
	}
}

func scanNode(_ *cobra.Command, args []string) {
	u, err := stdurl.Parse(args[0])
	checkf(err, "invalid URL")
	port, err := strconv.ParseUint(u.Port(), 10, 64)
	checkf(err, "invalid port")

	client := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(args[0], "v3"))
	client.Client.Timeout = time.Hour
	peer, err := healing.ScanNode(context.Background(), client)
	check(err)

	p2pPort := port - uint64(config.PortOffsetAccumulateApi) + uint64(config.PortOffsetAccumulateP2P)
	tcp, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", u.Hostname(), p2pPort))
	check(err)
	udp, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/udp/%d/quic", u.Hostname(), p2pPort))
	check(err)
	peer.Addresses = []multiaddr.Multiaddr{tcp, udp}

	if outputJSON {
		fmt.Fprintln(os.Stderr, peer.ID)
		check(json.NewEncoder(os.Stdout).Encode(peer))
		return
	}

	fmt.Printf("  %v\n", peer)
}

func networkStatus(_ *cobra.Command, args []string) {
	// slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
	// 	Level: slog.LevelDebug,
	// })))

	ctx := cmdutil.ContextForMainProcess(context.Background())

	public := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(args[0], "v3"))
	ni, err := public.NodeInfo(ctx, api.NodeInfoOptions{})
	check(err)

	ns, err := public.NetworkStatus(ctx, api.NetworkStatusOptions{})
	check(err)
	router := routing.NewRouter(routing.RouterOptions{Initial: ns.Routing})

	// Check for a cached scan
	var network *healing.NetworkInfo
	cachedScanPath := cachedScan
	if cachedScanPath == "" {
		cachedScanPath = filepath.Join(currentUser.HomeDir, ".accumulate", "cache", strings.ToLower(ni.Network+".json"))
	}
	data, err := os.ReadFile(cachedScanPath)
	switch {
	case err == nil:
		// Load the cached scan
		check(json.Unmarshal(data, &network))

	case errors.Is(err, fs.ErrNotExist):
		// Scan the network
		network, err = healing.ScanNetwork(ctx, public)
		check(err)

		// Save it
		data, err = json.Marshal(network)
		check(err)
		check(os.WriteFile(cachedScanPath, data, 0600))

	default:
		check(err)
	}

	// Map keys to peers
	peerByKeyHash := map[[32]byte]peer.ID{}
	addrByKeyHash := map[[32]byte][]multiaddr.Multiaddr{}
	for _, part := range network.Peers {
		for _, peer := range part {
			kh := sha256.Sum256(peer.Key[:])
			peerByKeyHash[kh] = peer.ID

			c, err := multiaddr.NewComponent("p2p", peer.ID.String())
			check(err)
			for _, addr := range peer.Addresses {
				addrByKeyHash[kh] = append(addrByKeyHash[kh], addr.Encapsulate(c))
			}
		}
	}

	mu := new(sync.Mutex)
	wg := new(sync.WaitGroup)

	var nodes []*nodeStatus
	nodeByKeyHash := map[[32]byte]*nodeStatus{}
	seenComet := map[cmtp2p.ID]bool{}
	var cometPeers []coretypes.Peer

	// Find a host for each validator
	fmt.Fprintln(os.Stderr, "Identifying validators...")
	work(mu, wg, func() {
		for _, val := range ns.Network.Validators {
			slog.DebugContext(ctx, "Checking validator", "id", logging.AsHex(val.PublicKeyHash).Slice(0, 4), "operator", val.Operator)
			st := new(nodeStatus)
			st.PeerID = peerByKeyHash[val.PublicKeyHash]
			st.Key = val.PublicKey
			st.Operator = val.Operator
			st.Parts = map[string]*nodePartStatus{}
			for _, part := range val.Partitions {
				if !part.Active {
					continue
				}
				id := strings.ToLower(part.ID)
				if _, ok := st.Parts[id]; !ok {
					st.Parts[id] = &nodePartStatus{Validator: true}
				}
			}
			if len(st.Parts) == 0 {
				slog.DebugContext(ctx, "Validator is inactive", "id", logging.AsHex(val.PublicKeyHash).Slice(0, 4), "operator", val.Operator)
				continue
			}

			nodeByKeyHash[val.PublicKeyHash] = st
			nodes = append(nodes, st)

			for _, addr := range addrByKeyHash[val.PublicKeyHash] {
				slog.DebugContext(ctx, "Checking validator address", "id", logging.AsHex(val.PublicKeyHash).Slice(0, 4), "operator", val.Operator, "address", addr)

				var host string
				multiaddr.ForEach(addr, func(c multiaddr.Component) bool {
					switch c.Protocol().Code {
					case multiaddr.P_DNS,
						multiaddr.P_DNS4,
						multiaddr.P_DNS6,
						multiaddr.P_IP4,
						multiaddr.P_IP6:
						host = c.Value()
						return false
					}
					return true
				})
				if host == "" {
					continue
				}

				// Check if we can connect to Tendermint
				base := fmt.Sprintf("http://%s:16592", host)
				c, err := http.New(base, base+"/ws")
				if err != nil {
					slog.DebugContext(ctx, "Error while creating consensus client", "error", err, "host", host)
					continue
				}

				status := promise.Call(maybe(func() (*coretypes.ResultStatus, bool) {
					ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
					defer cancel()

					status, err := c.Status(ctx)
					if err != nil {
						slog.DebugContext(ctx, "Error while querying consensus status", "error", err, "host", host)
						return nil, false
					}
					return status, true
				}))
				waitFor(wg, promise.SyncThen(mu, status, done(func(status *coretypes.ResultStatus) {
					st.Host = host
					seenComet[status.NodeInfo.ID()] = true
				})))

				netInfo := promise.Call(maybe(func() (*coretypes.ResultNetInfo, bool) {
					ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
					defer cancel()

					netInfo, err := c.NetInfo(ctx)
					if err != nil {
						slog.DebugContext(ctx, "Error while querying consensus status", "error", err, "host", host)
						return nil, false
					}
					return netInfo, true
				}))
				waitFor(wg, promise.SyncThen(mu, netInfo, done(func(netInfo *coretypes.ResultNetInfo) {
					cometPeers = append(cometPeers, netInfo.Peers...)
				})))
				break
			}
		}
	})

	// Use consensus peers to find hosts
	fmt.Fprintln(os.Stderr, "Checking peers...")
	work(mu, wg, func() {
		for len(cometPeers) > 0 {
			scan := cometPeers
			cometPeers = nil
			for _, peer := range scan {
				if seenComet[peer.NodeInfo.ID()] {
					continue
				}
				seenComet[peer.NodeInfo.ID()] = true
				slog.DebugContext(ctx, "Checking peer", "comet-id", peer.NodeInfo.ID(), "remote-ip", peer.RemoteIP)

				c, err := httpClientForPeer(peer, 0)
				if err != nil {
					slog.DebugContext(ctx, "Error while creating consensus client", "error", err, "host", peer.RemoteIP)
					continue
				}

				netInfo := promise.Call(maybe(func() (*coretypes.ResultNetInfo, bool) {
					ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
					defer cancel()

					netInfo, err := c.NetInfo(ctx)
					if err != nil {
						slog.DebugContext(ctx, "Error while querying consensus network info", "error", err, "host", peer.RemoteIP)
						return nil, false
					}
					return netInfo, true
				}))
				waitFor(wg, promise.SyncThen(mu, netInfo, done(func(netInfo *coretypes.ResultNetInfo) {
					for _, peer := range netInfo.Peers {
						if !seenComet[peer.NodeInfo.ID()] {
							cometPeers = append(cometPeers, peer)
						}
					}
				})))

				peer := peer
				status := promise.Call(maybe(func() (*coretypes.ResultStatus, bool) {
					ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
					defer cancel()

					status, err := c.Status(ctx)
					if err != nil {
						slog.DebugContext(ctx, "Error while querying consensus status", "error", err, "host", peer.RemoteIP)
						return nil, false
					}
					return status, true
				}))
				node := promise.SyncThen(mu, status, definitely(func(status *coretypes.ResultStatus) *nodeStatus {
					kh := sha256.Sum256(status.ValidatorInfo.PubKey.Bytes())
					node, ok := nodeByKeyHash[kh]
					if ok {
						node.Host = peer.RemoteIP
						return node
					}

					node = new(nodeStatus)
					node.Key = status.ValidatorInfo.PubKey.Bytes()
					node.Parts = map[string]*nodePartStatus{}
					nodeByKeyHash[kh] = node
					nodes = append(nodes, node)
					node.Host = peer.RemoteIP
					return node
				}))
				waitFor(wg, promise.Then(node, func(node *nodeStatus) promise.Result[any] {
					ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
					defer cancel()

					c := jsonrpc.NewClient(fmt.Sprintf("http://%s:16595/v3", node.Host))
					nodeInfo, err := c.NodeInfo(ctx, api.NodeInfoOptions{})
					if err != nil {
						slog.DebugContext(ctx, "Error while querying node info", "error", err, "host", node.Host)
						return promise.ErrorOf[any](err)
					}

					node.PeerID = nodeInfo.PeerID
					return promise.ValueOf[any](nil)
				}))
			}
		}
	})

	// Get version
	fmt.Fprintln(os.Stderr, "Checking versions...")
	work(mu, wg, func() {
		for _, node := range nodes {
			if node.Host == "" {
				continue
			}

			c, err := v2.New(fmt.Sprintf("http://%s:16595/v2", node.Host))
			if err != nil {
				continue
			}

			node := node
			version := promise.Call(maybe(func() (string, bool) {
				r, err := c.Version(ctx)
				if err != nil {
					return "", false
				}
				data, err := json.Marshal(r.Data)
				if err != nil {
					return "", false
				}

				version := new(v2.VersionResponse)
				err = json.Unmarshal(data, version)
				if err != nil {
					return "", false
				}
				return version.Version, true
			}))
			waitFor(wg, promise.SyncThen(mu, version, done(func(s string) {
				node.Version = s
			})))
		}
	})

	// Check API v3
	fmt.Fprintln(os.Stderr, "Checking API v3...")
	work(mu, wg, func() {
		for _, node := range nodes {
			if node.Host == "" || node.PeerID == "" {
				continue
			}

			kh := sha256.Sum256(node.Key)
			slog.DebugContext(ctx, "Checking API v3", "operator", node.Operator, "id", logging.AsHex(kh).Slice(0, 4), "host", node.Host)
			addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/dns/%s/tcp/16593/p2p/%v", node.Host, node.PeerID))
			check(err)

			// Check API v3
			p2p, err := p2p.New(p2p.Options{
				Network:        ni.Network,
				BootstrapPeers: []multiaddr.Multiaddr{},
			})
			check(err)
			dialer := p2p.DialNetwork()

			node := node

			connect := promise.Call(maybe(func() (any, bool) {
				ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()

				_, err = dialer.Dial(ctx, addr.Encapsulate(api.ServiceTypeNode.Address().Multiaddr()))
				if err != nil {
					slog.DebugContext(ctx, "Error while dialing libp2p", "error", err, "address", addr)
				}
				return nil, err == nil
			}))
			didConnect := promise.SyncThen(mu, connect, func(any) promise.Result[any] {
				node.API.ConnectV3 = true
				return promise.ValueOf[any](nil)
			})
			query := promise.Then(didConnect, func(any) promise.Result[*api.NodeInfo] {
				ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()

				mc := &message.Client{Transport: &message.RoutedTransport{
					Network: ni.Network,
					Dialer:  dialer,
					Router:  routing.MessageRouter{Router: router},
				}}
				ni, err = mc.NodeInfo(ctx, api.NodeInfoOptions{PeerID: node.PeerID})
				if err != nil {
					slog.DebugContext(ctx, "Error while querying via libp2p", "error", err, "address", addr)
					return promise.ErrorOf[*api.NodeInfo](nil)
				}
				return promise.ValueOf(ni)
			})
			waitFor(wg, promise.SyncThen(mu, query, done(func(ni *api.NodeInfo) {
				if ni.PeerID != node.PeerID {
					slog.InfoContext(ctx, "Got wrong peer ID", "expected", node.PeerID, "actual", ni.PeerID, "address", addr)
				}
				node.API.QueryV3 = true
			})))
		}
	})

	// Check consensus
	fmt.Fprintln(os.Stderr, "Checking consensus...")
	work(mu, wg, func() {
		for _, node := range nodes {
			if node.Host == "" {
				continue
			}

			// Query Tendermint
			for _, port := range []int{16592, 16692} {
				base := fmt.Sprintf("http://%s:%d", node.Host, port)
				kh := sha256.Sum256(node.Key)
				slog.DebugContext(ctx, "Checking consensus", "operator", node.Operator, "id", logging.AsHex(kh).Slice(0, 4), "host", base)

				c, err := http.New(base, base+"/ws")
				if err != nil {
					continue
				}

				node := node
				status := promise.Call(try(func() (*coretypes.ResultStatus, error) {
					ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
					defer cancel()

					return c.Status(ctx)
				}))
				catchAndLog(ctx, status, "Error while querying consensus status", "host", node.Host)
				part := promise.SyncThen(mu, status, func(status *coretypes.ResultStatus) promise.Result[*nodePartStatus] {
					part := status.NodeInfo.Network
					if i := strings.LastIndexByte(part, '.'); i >= 0 {
						part = part[i+1:]
					}
					part = strings.ToLower(part)

					st, ok := node.Parts[part]
					if !ok {
						st = new(nodePartStatus)
						node.Parts[part] = st
					}

					st.Height = uint64(status.SyncInfo.LatestBlockHeight)
					return promise.ValueOf(st)
				})

				waitFor(wg, promise.Then(part, func(part *nodePartStatus) promise.Result[any] {
					z, err := nodeIsZombie(ctx, c)
					if err != nil {
						return promise.ErrorOf[any](err)
					}
					part.Zombie = z
					return promise.ValueOf[any](nil)
				}))
			}
		}
	})

	if outputJSON {
		check(json.NewEncoder(os.Stdout).Encode(nodes))
		return
	}

	header := []string{"Validator ID", "Operator", "Host", "Version", "API v3", "Directory"}
	parts := []string{"Directory"}
	for _, part := range ns.Network.Partitions {
		if part.Type == protocol.PartitionTypeDirectory {
			continue
		}
		header = append(header, part.ID)
		parts = append(parts, part.ID)
	}

	tw := tablewriter.NewWriter(os.Stdout)
	tw.SetHeader(header)
	defer tw.Render()

	good := color.GreenString("✔")
	bad := color.RedString("🗴")
	unknown := color.RedString("unknown")
	isVal := color.HiBlueString("🔑")
	isDead := color.RedString("💀")
	nbsp := "\u00A0"

	for _, node := range nodes {
		kh := sha256.Sum256(node.Key)
		values := []string{
			fmt.Sprintf("%x", kh[:4]),
			sprintValueOr(node.Operator, ""),
			sprintValueOr(node.Host, unknown),
			sprintValueOr(node.Version, unknown),
		}

		if node.Host == "" || node.PeerID == "" {
			values = append(values, "")
		} else {
			values = append(values, pickStr(node.API.ConnectV3, good, bad)+nbsp+pickStr(node.API.QueryV3, good, bad))
		}

		for _, part := range parts {
			st := node.Parts[strings.ToLower(part)]
			if st == nil {
				st = new(nodePartStatus)
			}

			bad := unknown
			if !st.Validator {
				bad = ""
			}

			values = append(values, pickStr(st.Validator, isVal, nbsp)+nbsp+sprintValueOr(st.Height, bad)+nbsp+pickStr(st.Zombie, isDead, nbsp))
		}

		tw.Append(values)
	}
}

func pickStr(ok bool, good, bad string) string {
	if ok {
		return good
	}
	return bad
}

func sprintValueOr[T comparable](v T, empty string) string {
	var z T
	if v == z {
		return empty
	}
	return fmt.Sprint(v)
}

func httpClientForPeer(peer coretypes.Peer, offset config.PortOffset) (*http.HTTP, error) {
	// peer.node_info.listen_addr should include the Tendermint P2P port
	i := strings.IndexByte(peer.NodeInfo.ListenAddr, ':')
	if i < 0 {
		return nil, errors.New("invalid listen address")
	}

	// Convert the port number to a string
	port, err := strconv.ParseUint(peer.NodeInfo.ListenAddr[i+1:], 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid port: %v", err)
	}

	// Calculate the RPC port from the P2P port
	port = port - uint64(config.PortOffsetTendermintP2P) + uint64(config.PortOffsetTendermintRpc) + uint64(offset)

	// Construct the address from peer.remote_ip and the calculated port
	addr := fmt.Sprintf("http://%s:%d", peer.RemoteIP, port)

	// Create a new client
	return http.New(addr, addr+"/ws")
}

func nodeIsZombie(ctx context.Context, c client.MempoolClient) (bool, error) {
	txn := new(protocol.Transaction)
	txn.Body = new(protocol.BurnCredits)
	txn.Header.Principal = url.MustParse("garbage")
	txn.Header.Metadata = make([]byte, 32)
	_, err := rand.Read(txn.Header.Metadata)
	if err != nil {
		return false, err
	}

	env := new(messaging.Envelope)
	env.Transaction = []*protocol.Transaction{txn}
	b, err := env.MarshalBinary()
	if err != nil {
		return false, err
	}

	_, err = c.CheckTx(ctx, b)
	var rpcErr *types.RPCError
	if errors.As(err, &rpcErr) && rpcErr.Data == "panicked" {
		return true, nil
	}
	return false, err
}
