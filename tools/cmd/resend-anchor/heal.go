// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var healCmd = &cobra.Command{
	Use:  "heal [network]",
	Args: cobra.ExactArgs(1),
	Run:  heal,
}

func init() {
	cmd.AddCommand(healCmd)
	healCmd.Flags().BoolVar(&healFlag.Continuous, "continuous", false, "Run healing in a loop every second")
}

var healFlag = struct {
	Continuous bool
}{}

func heal(_ *cobra.Command, args []string) {
	ctx, cancel, _ := api.ContextWithBatchData(context.Background())
	defer cancel()

	C2 := jsonrpc.NewClient(args[0])
	apiNode, err := C2.NodeInfo(ctx, api.NodeInfoOptions{})
	checkf(err, "query node info")

	status, err := C2.NetworkStatus(ctx, api.NetworkStatusOptions{})
	checkf(err, "query network status")

	node, err := p2p.New(p2p.Options{
		Network:        apiNode.Network,
		BootstrapPeers: api.BootstrapServers,
	})
	checkf(err, "start p2p node")
	defer func() { _ = node.Close() }()

	fmt.Printf("We are %v\n", node.ID())

	router := new(routing.MessageRouter)
	C := &message.Client{
		Transport: &message.RoutedTransport{
			Network: apiNode.Network,
			Dialer:  node.DialNetwork(),
			Router:  router,
		},
	}
	router.Router, err = routing.NewStaticRouter(status.Routing, nil)
	check(err)

	peers := getPeers(C2, ctx)

heal:
	// Heal BVN -> DN
	for _, part := range status.Network.Partitions {
		if part.Type != protocol.PartitionTypeBlockValidator {
			continue
		}

		partUrl := protocol.PartitionUrl(part.ID)
		ledger := getAccount[*protocol.AnchorLedger](C2, ctx, partUrl.JoinPath(protocol.AnchorPool))
		partLedger := ledger.Anchor(protocol.DnUrl())

		for i, txid := range ledger.Anchor(protocol.DnUrl()).Pending {
			healAnchor(C, C2, ctx, protocol.DnUrl(), partUrl, txid, partLedger.Delivered+1+uint64(i), peers[protocol.Directory])
		}
	}

	// Heal DN -> BVN, DN -> DN
	{
		ledger := getAccount[*protocol.AnchorLedger](C2, ctx, protocol.DnUrl().JoinPath(protocol.AnchorPool))

		for _, part := range status.Network.Partitions {
			partUrl := protocol.PartitionUrl(part.ID)
			partLedger := ledger.Anchor(partUrl)
			for i, txid := range ledger.Anchor(partUrl).Pending {
				healAnchor(C, C2, ctx, partUrl, protocol.DnUrl(), txid, partLedger.Delivered+1+uint64(i), peers[part.ID])
			}
		}
	}

	// Heal continuously?
	if healFlag.Continuous {
		time.Sleep(time.Second)
		goto heal
	}
}

func getPeers(C2 *jsonrpc.Client, ctx context.Context) map[string]map[peer.ID][32]byte {
	apiNode, err := C2.NodeInfo(ctx, api.NodeInfoOptions{})
	checkf(err, "query node info")

	status, err := C2.NetworkStatus(ctx, api.NetworkStatusOptions{})
	checkf(err, "query network status")

	hash2key := map[[32]byte][32]byte{}
	for _, val := range status.Network.Validators {
		hash2key[val.PublicKeyHash] = *(*[32]byte)(val.PublicKey)
	}

	peers := map[string]map[peer.ID][32]byte{}
	for _, part := range status.Network.Partitions {
		peers[part.ID] = map[peer.ID][32]byte{}

		fmt.Printf("Getting peers for %s\n", part.ID)
		find := api.FindServiceOptions{
			Network: apiNode.Network,
			Service: api.ServiceTypeConsensus.AddressFor(part.ID),
		}
		res, err := C2.FindService(ctx, find)
		checkf(err, "find %s on %s", find.Service.String(), find.Network)

		for _, peer := range res {
			fmt.Printf("Getting identity of %v\n", peer.PeerID)
			info, err := C2.ConsensusStatus(ctx, api.ConsensusStatusOptions{NodeID: peer.PeerID.String(), Partition: part.ID})
			if err != nil {
				fmt.Printf("%+v\n", err)
				continue
			}

			key, ok := hash2key[info.ValidatorKeyHash]
			if !ok {
				continue // Not a validator
			}

			peers[part.ID][peer.PeerID] = key
		}
	}
	return peers
}

func getLedger(c *client.Client, part *url.URL) *protocol.AnchorLedger { //nolint:unused
	ledger := new(protocol.AnchorLedger)
	res := new(client.ChainQueryResponse)
	res.Data = ledger
	req := new(client.GeneralQuery)
	req.Url = part.JoinPath(protocol.AnchorPool)
	err := c.RequestAPIv2(context.Background(), "query", req, res)
	checkf(err, "query %s anchor ledger", part)
	return ledger
}

func healTx(g *core.GlobalValues, nodes map[string][]*NodeData, netClient *client.Client, srcUrl, dstUrl *url.URL, txid *url.TxID) { //nolint:unused
	// dstId, _ := protocol.ParsePartitionUrl(dstUrl)
	srcId, _ := protocol.ParsePartitionUrl(srcUrl)

	// Query the transaction
	res, err := netClient.QueryTx(context.Background(), &client.TxnQuery{TxIdUrl: txid})
	if err != nil {
		log.Printf("Failed to query %v: %v\n", txid, err)
		return
	}

	// Check if there are already enough transactions
	if uint64(len(res.Status.AnchorSigners)) >= g.ValidatorThreshold(srcId) {
		return // Already have enough signers
	}

	fmt.Printf("Healing anchor %v\n", txid)

	// Mark which nodes have signed
	signed := map[[32]byte]bool{}
	for _, s := range res.Status.AnchorSigners {
		signed[*(*[32]byte)(s)] = true
	}

	// // Make a client for the destination
	// dstClient := nodes[strings.ToLower(dstId)][0].AccumulateAPIForUrl(dstUrl)

	// Get a signature from each node that hasn't signed
	for _, node := range nodes[strings.ToLower(srcId)] {
		if signed[*(*[32]byte)(node.Info.PublicKey)] {
			continue
		}

		// Make a client for the source
		srcClient := node.AccumulateAPIForUrl(srcUrl)

		// Query and execute the anchor
		querySynthAndExecute(srcClient, netClient, srcUrl, dstUrl, res.Status.SequenceNumber, false)
	}
}

func getAccount[T protocol.Account](C api.Querier, ctx context.Context, u *url.URL) T {
	var v T
	_, err := api.Querier2{Querier: C}.QueryAccountAs(ctx, u, nil, &v)
	checkf(err, "get %v", u)
	return v
}

func healAnchor(C *message.Client, C2 *jsonrpc.Client, ctx context.Context, srcUrl, dstUrl *url.URL, txid *url.TxID, seqNum uint64, peers map[peer.ID][32]byte) {
	fmt.Printf("Healing anchor %v\n", txid)

	dstId, ok := protocol.ParsePartitionUrl(dstUrl)
	if !ok {
		panic("not a partition: " + dstUrl.String())
	}

	// Query the transaction
	res, err := api.Querier2{Querier: C2}.QueryTransaction(ctx, txid, nil)
	checkf(err, "get %v", txid)

	// Mark which nodes have signed
	signed := map[[32]byte]bool{}
	for _, sigs := range res.Signatures.Records {
		for _, sig := range sigs.Signatures.Records {
			msg, ok := sig.Message.(*messaging.BlockAnchor)
			if !ok {
				continue
			}
			signed[*(*[32]byte)(msg.Signature.GetPublicKey())] = true
		}
	}

	env := new(messaging.Envelope)
	env.Transaction = []*protocol.Transaction{res.Message.Transaction}

	// Get a signature from each node that hasn't signed
	var bad []peer.ID
	for peer, key := range peers {
		if signed[key] {
			continue
		}

		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		fmt.Printf("Querying %v for %v\n", peer, txid)
		res, err := C.ForPeer(peer).Private().Sequence(ctx, srcUrl.JoinPath(protocol.AnchorPool), dstUrl, seqNum)
		if err != nil {
			fmt.Printf("%+v\n", err)
			bad = append(bad, peer)
			continue
		}

		for _, sigs := range res.Signatures.Records {
			for _, sig := range sigs.Signatures.Records {
				msg, ok := sig.Message.(*messaging.SignatureMessage)
				if !ok {
					continue
				}
				env.Signatures = append(env.Signatures, msg.Signature)
			}
		}
	}

	for _, peer := range bad {
		fmt.Printf("Removing bad peer %v from the list of candidates\n", peer)
		delete(peers, peer)
	}

	if len(env.Signatures) == 0 {
		fmt.Println("Nothing to send")
		return
	}

	fmt.Printf("Submitting %d signatures\n", len(env.Signatures))
	addr := api.ServiceTypeSubmit.AddressFor(dstId).Multiaddr()
	sub, err := C.ForAddress(addr).Submit(ctx, env, api.SubmitOptions{})
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, sub := range sub {
		if !sub.Success {
			fmt.Println(sub.Message)
		}
	}
}
