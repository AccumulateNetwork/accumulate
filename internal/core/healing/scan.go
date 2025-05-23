// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package healing

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/p2p"
)

func (p *PeerInfo) String() string {
	if p.Operator != nil {
		return fmt.Sprintf("%v (%v)", p.Operator, p.ID)
	}
	return p.ID.String()
}

type NetworkInfo struct {
	Status *api.NetworkStatus  `json:"status"`
	ID     string              `json:"id"`
	Peers  map[string]PeerList `json:"peers"`
}

type PeerList map[peer.ID]*PeerInfo

func (i *NetworkInfo) PeerByID(id peer.ID) *PeerInfo {
	if i == nil {
		return nil
	}
	for _, part := range i.Peers {
		if p, ok := part[id]; ok {
			return p
		}
	}
	return nil
}

func (l PeerList) MarshalJSON() ([]byte, error) {
	m := make(map[string]*PeerInfo, len(l))
	for id, info := range l {
		m[id.String()] = info
	}
	return json.Marshal(m)
}

func (l *PeerList) UnmarshalJSON(data []byte) error {
	var m map[string]*PeerInfo
	err := json.Unmarshal(data, &m)
	if err != nil {
		return err
	}
	*l = make(PeerList, len(m))
	for s, info := range m {
		id, err := peer.Decode(s)
		if err != nil {
			return err
		}
		info.ID = id
		(*l)[id] = info
	}
	return nil
}

type ScanServices = interface {
	api.NodeService
	api.ConsensusService
	api.NetworkService
}

func ScanNetwork(ctx context.Context, endpoint ScanServices) (*NetworkInfo, error) {
	slog.Info("Scanning the network")
	ctx, cancel, _ := api.ContextWithBatchData(ctx)
	defer cancel()

	epNodeInfo, err := endpoint.NodeInfo(ctx, api.NodeInfoOptions{})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("query endpoint node info: %w", err)
	}

	netStatus, err := endpoint.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("query network status: %w", err)
	}

	hash2key := map[[32]byte][32]byte{}
	for _, val := range netStatus.Network.Validators {
		hash2key[val.PublicKeyHash] = *(*[32]byte)(val.PublicKey)
	}

	peers := map[string]PeerList{}
	for _, part := range netStatus.Network.Partitions {
		partPeers := PeerList{}
		peers[strings.ToLower(part.ID)] = partPeers

		slog.InfoContext(ctx, "Finding peers for", "partition", part.ID)
		find := api.FindServiceOptions{
			Network: epNodeInfo.Network,
			Service: api.ServiceTypeConsensus.AddressFor(part.ID),
			Timeout: 10 * time.Second,
		}
		res, err := endpoint.FindService(ctx, find)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("find %s: %w", find.Service.String(), err)
		}

		for _, peer := range res {
			slog.InfoContext(ctx, "Getting identity of", "peer", peer.PeerID)
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			// This is a hack to circumvent libp2p's issues
			client := jsonrpcClientForPeer(peer.Addresses)
			if client == nil {
				slog.ErrorContext(ctx, "Unable to determine address of", "peer", peer.PeerID)
				continue
			}

			info, err := client.ConsensusStatus(ctx, api.ConsensusStatusOptions{NodeID: peer.PeerID.String(), Partition: part.ID})
			if err != nil {
				slog.ErrorContext(ctx, "Query failed", "error", err)
				continue
			}

			key, ok := hash2key[info.ValidatorKeyHash]
			if !ok {
				continue // Not a validator
			}
			pi := &PeerInfo{
				ID:        peer.PeerID,
				Status:    info,
				Key:       key,
				Addresses: peer.Addresses,
			}
			partPeers[peer.PeerID] = pi

			_, val, ok := netStatus.Network.ValidatorByHash(info.ValidatorKeyHash[:])
			if ok {
				pi.Operator = val.Operator
			}
		}
	}

	return &NetworkInfo{
		Status: netStatus,
		ID:     epNodeInfo.Network,
		Peers:  peers,
	}, nil
}

// This is a hack to circumvent libp2p's issues
func jsonrpcClientForPeer(addrs []p2p.Multiaddr) *jsonrpc.Client {
	for _, addr := range addrs {
		var host string
		multiaddr.ForEach(addr, func(c multiaddr.Component) bool {
			switch c.Protocol().Code {
			case multiaddr.P_IP4, multiaddr.P_IP6, multiaddr.P_DNS:
				host = c.Value()
				return false
			}
			return true
		})
		if host != "" {
			return jsonrpc.NewClient(fmt.Sprintf("http://%s:16595/v3", host))
		}
	}
	return nil
}

func ScanNode(ctx context.Context, endpoint ScanServices) (*PeerInfo, error) {
	ctx, cancel, _ := api.ContextWithBatchData(ctx)
	defer cancel()

	nodeInfo, err := endpoint.NodeInfo(ctx, api.NodeInfoOptions{})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("query node info: %w", err)
	}

	netStatus, err := endpoint.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("query network status: %w", err)
	}

	hash2key := map[[32]byte][32]byte{}
	for _, val := range netStatus.Network.Validators {
		hash2key[val.PublicKeyHash] = *(*[32]byte)(val.PublicKey)
	}

	slog.InfoContext(ctx, "Getting identity of", "peer", nodeInfo.PeerID)
	info, err := endpoint.ConsensusStatus(ctx, api.ConsensusStatusOptions{})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("query consensus status: %w", err)
	}

	key, ok := hash2key[info.ValidatorKeyHash]
	if !ok {
		return nil, errors.UnknownError.With("not a validator")
	}

	pi := &PeerInfo{
		ID:     nodeInfo.PeerID,
		Status: info,
		Key:    key,
	}

	_, val, ok := netStatus.Network.ValidatorByHash(info.ValidatorKeyHash[:])
	if ok {
		pi.Operator = val.Operator
	}

	return pi, nil
}
