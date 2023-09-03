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
	"strconv"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/healing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/exp/slog"
)

var cmdHealAnchor = &cobra.Command{
	Use:   "anchor [network] [txid or part→part (optional) [sequence number (optional)]]",
	Short: "Heal anchoring",
	Args:  cobra.RangeArgs(1, 3),
	Run:   healAnchor,
}

func init() {
	cmdHeal.AddCommand(cmdHealAnchor)
	cmdHealAnchor.Flags().BoolVar(&healContinuous, "continuous", false, "Run healing in a loop every second")
	cmdHealAnchor.Flags().StringVar(&cachedScan, "cached-scan", "", "A cached network scan")
	cmdHealAnchor.Flags().BoolVarP(&pretend, "pretend", "n", false, "Do not submit envelopes, only scan")
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

	fmt.Fprintf(os.Stderr, "We are %v\n", node.ID())

	fmt.Fprintln(os.Stderr, "Waiting for addresses")
	time.Sleep(time.Second)

	// We should be able to use only the p2p client but it doesn't work well for
	// some reason
	///C1 := jsonrpc.NewClient(api.ResolveWellKnownEndpoint(networkID))
	C1 := jsonrpc.NewClient(api.ResolveWellKnownEndpoint("http://65.109.48.173:16695/v3"))
	C1.Client.Timeout = time.Hour

	// Use a hack dialer that uses the API for peer discovery
	router := new(routing.MessageRouter)
	C2 := &message.Client{
		Transport: &message.RoutedTransport{
			Network: networkID,
			Dialer:  &hackDialer{C1, node.DialNetwork(), map[string]peer.ID{}},
			Router:  router,
		},
	}

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

			err = healing.HealAnchor(ctx, C1, C2, net, r.Sequence.Source, r.Sequence.Destination, r.Sequence.Number, r.Message.Transaction, r.Signatures.Records, pretend)
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

		var seqNo uint64
		if len(args) > 2 {
			seqNo, err = strconv.ParseUint(args[2], 10, 64)
			check(err)
		}

		if seqNo > 0 {
			err = healing.HealAnchor(ctx, C1, C2, net, srcUrl, dstUrl, seqNo, nil, nil, pretend)
			check(err)
			return
		}

		ledger1 := getAccount[*protocol.AnchorLedger](C1, ctx, dstUrl.JoinPath(protocol.AnchorPool))
		ledger2 := ledger1.Anchor(srcUrl)
		for i, txid := range ledger2.Pending {
			res, err := api.Querier2{Querier: C1}.QueryTransaction(ctx, txid, nil)
			check(err)
			err = healing.HealAnchor(ctx, C1, C2, net, srcUrl, dstUrl, ledger2.Delivered+1+uint64(i), res.Message.Transaction, res.Signatures.Records, pretend)
			check(err)
		}
		return
	}

heal:
	for _, dst := range net.Status.Network.Partitions {
		dstUrl := protocol.PartitionUrl(dst.ID)
		dstLedger := getAccount[*protocol.AnchorLedger](C1, ctx, dstUrl.JoinPath(protocol.AnchorPool))

		for _, src := range net.Status.Network.Partitions {
			// Anchors are always from and/or to the DN
			if dst.Type != protocol.PartitionTypeDirectory && src.Type != protocol.PartitionTypeDirectory {
				continue
			}

			srcUrl := protocol.PartitionUrl(src.ID)
			src2dst := dstLedger.Partition(srcUrl)

			ids, txns := findPendingAnchors(ctx, C2, api.Querier2{Querier: C1}, srcUrl, dstUrl, true)
			src2dst.Pending = append(src2dst.Pending, ids...)

			for i, txid := range src2dst.Pending {
				if txid == nil {
					err = healing.HealAnchor(ctx, C1, C2, net, srcUrl, dstUrl, src2dst.Delivered+1+uint64(i), nil, nil, pretend)
					check(err)
					continue
				}

				var txn *protocol.Transaction
				var sigSets []*api.SignatureSetRecord
				res, err := api.Querier2{Querier: C1}.QueryTransaction(ctx, txid, nil)
				switch {
				case err == nil:
					txn = res.Message.Transaction
					sigSets = res.Signatures.Records
				case !errors.Is(err, errors.NotFound):
					//check to see if the message is sequence message
					res, err := api.Querier2{Querier: C1}.QueryMessage(ctx, txid, nil)
					if err != nil {
						slog.ErrorCtx(ctx, "Query message failed", "error", err)
						continue
					}

					seq, ok := res.Message.(*messaging.SequencedMessage)
					if !ok {
						slog.ErrorCtx(ctx, "Message receieved was not a sequenced message")
						continue
					}
					txm, ok := seq.Message.(*messaging.TransactionMessage)
					if !ok {
						slog.ErrorCtx(ctx, "Sequenced message does not contain a transaction message")
						continue
					}

					txn = txm.Transaction
					sigSets = res.Signatures.Records
				default:
					var ok bool
					txn, ok = txns[txid.Hash()]
					if !ok {
						check(err)
					}
				}
				err = healing.HealAnchor(ctx, C1, C2, net, srcUrl, dstUrl, src2dst.Delivered+1+uint64(i), txn, sigSets, pretend)
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
