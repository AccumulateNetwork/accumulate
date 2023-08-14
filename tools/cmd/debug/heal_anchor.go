// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/healing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdHealAnchor = &cobra.Command{
	Use:   "anchor [network] [txid or part→part (optional)]",
	Short: "Heal anchoring",
	Args:  cobra.RangeArgs(1, 2),
	Run:   healAnchor,
}

func init() {
	cmdHeal.AddCommand(cmdHealAnchor)
	cmdHealAnchor.Flags().BoolVar(&healContinuous, "continuous", false, "Run healing in a loop every second")
	cmdHealAnchor.Flags().StringVar(&cachedScan, "cached-scan", "", "A cached network scan")
	_ = cmdHealAnchor.MarkFlagFilename("cached-scan", ".json")
}

func healAnchor(_ *cobra.Command, args []string) {
	ctx, cancel, _ := api.ContextWithBatchData(context.Background())
	defer cancel()

	networkID := args[0]
	node, err := p2p.New(p2p.Options{
		Network:        networkID,
		BootstrapPeers: api.BootstrapServers,
	})
	checkf(err, "start p2p node")
	defer func() { _ = node.Close() }()

	fmt.Printf("We are %v\n", node.ID())

	// Wait for libp2p to do its thing
	time.Sleep(time.Second)

	// fmt.Println("Waiting for a live network service")
	// svcAddr, err := api.ServiceTypeNetwork.AddressFor(protocol.Directory).MultiaddrFor(networkID)
	// check(err)
	// check(node.WaitForService(context.Background(), svcAddr))

	router := new(routing.MessageRouter)
	C2 := &message.Client{
		Transport: &message.RoutedTransport{
			Network: networkID,
			Dialer:  node.DialNetwork(),
			Router:  router,
		},
	}

	// ns, err := C2.NetworkStatus(context.Background(), api.NetworkStatusOptions{})
	// check(err)
	// router.Router, err = routing.NewStaticRouter(ns.Routing, nil)
	// check(err)

	// We should be able to use only the p2p client but it doesn't work well for
	// some reason
	C1 := jsonrpc.NewClient(api.ResolveWellKnownEndpoint(networkID))

	var net *healing.NetworkInfo
	if cachedScan == "" {
		net, err = healing.ScanNetwork(ctx, C1)
		check(err)
	} else {
		data, err := os.ReadFile(cachedScan)
		check(err)
		check(json.Unmarshal(data, &net))
	}

	if len(args) > 1 {
		txid, err := url.ParseTxID(args[1])
		if err == nil {
			r, err := api.Querier2{Querier: C1}.QueryTransaction(ctx, txid, nil)
			check(err)
			if r.Sequence == nil {
				fatalf("%v is not sequenced", txid)
			}

			err = healing.HealAnchor(ctx, C1, C2, net, r.Sequence.Source, r.Sequence.Destination, r.Sequence.Number, r)
			check(err)
			return
		}

		parts := strings.Split(args[1], "→")
		if len(parts) != 2 {
			fatalf("invalid transaction ID or sequence specifier: %q", args[1])
		}
		srcId, dstId := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		srcUrl := protocol.PartitionUrl(srcId)
		dstUrl := protocol.PartitionUrl(dstId)

		ledger1 := getAccount[*protocol.AnchorLedger](C1, ctx, dstUrl.JoinPath(protocol.AnchorPool))
		ledger2 := ledger1.Anchor(srcUrl)
		for i, txid := range ledger2.Pending {
			res, err := api.Querier2{Querier: C1}.QueryTransaction(ctx, txid, nil)
			check(err)
			err = healing.HealAnchor(ctx, C1, C2, net, srcUrl, dstUrl, ledger2.Delivered+1+uint64(i), res)
			check(err)
		}
		return
	}

heal:
	// Heal BVN -> DN
	for _, part := range net.Status.Network.Partitions {
		if part.Type != protocol.PartitionTypeBlockValidator {
			continue
		}

		partUrl := protocol.PartitionUrl(part.ID)
		ledger := getAccount[*protocol.AnchorLedger](C1, ctx, partUrl.JoinPath(protocol.AnchorPool))
		partLedger := ledger.Anchor(protocol.DnUrl())

		for i, txid := range partLedger.Pending {
			res, err := api.Querier2{Querier: C1}.QueryTransaction(ctx, txid, nil)
			check(err)
			err = healing.HealAnchor(ctx, C1, C2, net, protocol.DnUrl(), partUrl, partLedger.Delivered+1+uint64(i), res)
			check(err)
		}
	}

	// Heal DN -> BVN, DN -> DN
	{
		ledger := getAccount[*protocol.AnchorLedger](C1, ctx, protocol.DnUrl().JoinPath(protocol.AnchorPool))

		for _, part := range net.Status.Network.Partitions {
			partUrl := protocol.PartitionUrl(part.ID)
			partLedger := ledger.Anchor(partUrl)
			for i, txid := range partLedger.Pending {
				res, err := api.Querier2{Querier: C1}.QueryTransaction(ctx, txid, nil)
				check(err)
				err = healing.HealAnchor(ctx, C1, C2, net, partUrl, protocol.DnUrl(), partLedger.Delivered+1+uint64(i), res)
				check(err)
			}
		}
	}

	// Heal continuously?
	if healContinuous {
		time.Sleep(time.Second)
		goto heal
	}
}

func getAccount[T protocol.Account](C api.Querier, ctx context.Context, u *url.URL) T {
	var v T
	_, err := api.Querier2{Querier: C}.QueryAccountAs(ctx, u, nil, &v)
	checkf(err, "get %v", u)
	return v
}
