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

	"github.com/multiformats/go-multiaddr"
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

var mainnetAddrs = func() []multiaddr.Multiaddr {
	s := []string{
		"/dns/apollo-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWAgrBYpWEXRViTnToNmpCoC3dvHdmR6m1FmyKjDn1NYpj",
		"/dns/yutu-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWDqFDwjHEog1bNbxai2dKSaR1aFvq2LAZ2jivSohgoSc7",
		"/dns/chandrayaan-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWHzjkoeAqe7L55tAaepCbMbhvNu9v52ayZNVQobdEE1RL",
		"/ip4/116.202.214.38/tcp/16593/p2p/12D3KooWBkJQiuvotpMemWBYfAe4ctsVHi7fLvT8RT83oXJ5dsgV",
		"/ip4/83.97.19.82/tcp/16593/p2p/12D3KooWHSbqS6K52d4ReauHAg4n8MFbAKkdEAae2fZXnzRYi9ce",
		"/ip4/206.189.97.165/tcp/16593/p2p/12D3KooWHyA7zgAVqGvCBBJejgvKzv7DQZ3LabJMWqmCQ9wFbT3o",
		"/ip4/144.76.105.23/tcp/16593/p2p/12D3KooWS2Adojqun5RV1Xy4k6vKXWpRQ3VdzXnW8SbW7ERzqKie",
		"/ip4/18.190.77.236/tcp/16593/p2p/12D3KooWP1d9vUJCzqX5bTv13tCHmVssJrgK3EnJCC2C5Ep2SXbS",
		"/ip4/3.28.207.55/tcp/16593/p2p/12D3KooWEzhg3CRvC3xdrUBFsWETF1nG3gyYfEjx4oEJer95y1Rk",
		"/ip4/38.135.195.81/tcp/16593/p2p/12D3KooWDWCHGAyeUWdP8yuuSYvMoUfaPoGu4p3gJb51diqNQz6j",
		// "/ip4/50.17.246.3/tcp/16593/p2p/12D3KooWKkNsxkHJqvSje2viyqKVxtqvbTpFrbASD3q1uv6td1pW",
		"/dns/validator-eu01.acme.sphereon.com/tcp/16593/p2p/12D3KooWKYTWKJ5jeuZmbbwiN7PoinJ2yJLoQtZyfWi2ihjBnSUR",
		"/ip4/35.86.120.53/tcp/16593/p2p/12D3KooWKJuspMDC5GXzLYJs9nHwYfqst9QAW4m5FakXNHVMNiq7",
		"/ip4/65.109.48.173/tcp/16593/p2p/12D3KooWHkUtGcHY96bNavZMCP2k5ps5mC7GrF1hBC1CsyGJZSPY",
		"/dns/accumulate.detroitledger.tech/tcp/16593/p2p/12D3KooWNe1QNh5mKAa8iAEP8vFwvmWFxaCLNcAdE1sH38Bz8sc9",
		"/ip4/3.135.9.97/tcp/16593/p2p/12D3KooWEQG3X528Ct2Kd3kxhv6WZDBqaAoEw7AKiPoK1NmWJgx1",
		// "/ip4/3.86.85.133/tcp/16593/p2p/12D3KooWJvReA1SuLkppyXKXq6fifVPLqvNtzsvPUqagVjvYe7qe",
		"/ip4/193.35.56.176/tcp/16593/p2p/12D3KooWJevZUFLqN7zAamDh2EEYNQZPvxGFwiFVyPXfuXZNjg1J",
		"/ip4/35.177.70.195/tcp/16593/p2p/12D3KooWPzpRp1UCu4nvXT9h8jKvmBmCADrMnoF72DrEbUrWrB2G",
		"/ip4/3.99.81.122/tcp/16593/p2p/12D3KooWLL5kAbD7nhv6CM9x9L1zjxSnc6hdMVKcsK9wzMGBo99X",
		"/ip4/34.219.75.234/tcp/16593/p2p/12D3KooWKHjS5nzG9dipBXn31pYEnfa8g5UzvkSYEsuiukGHzPvt",
		"/ip4/3.122.254.53/tcp/16593/p2p/12D3KooWRU8obVzgfw6TsUHjoy2FDD3Vd7swrPNTM7DMFs8JG4dx",
		"/ip4/35.92.228.236/tcp/16593/p2p/12D3KooWQqMqbyJ2Zay9KHeEDgDMAxQpKD1ypiBX5ByQAA2XpsZL",
		"/ip4/3.135.184.194/tcp/16593/p2p/12D3KooWHcxyiE3AGdPnhtj87tByfLnJZVR6mLefadWccbMByrBa",
		"/ip4/18.133.170.113/tcp/16593/p2p/12D3KooWFbWY2NhBEWTLHUCwwPmNHm4BoJXbojnrJJfuDCVoqrFY",
		// "/ip4/44.204.224.126/tcp/16593/p2p/12D3KooWAiJJxdgsB39up5h6fz6TSfBz4HsLKTFiBXUrbwA8o54m",
		"/ip4/35.92.21.90/tcp/16593/p2p/12D3KooWLTV3pTN2NbKeFeseCGHyMXuAkQv68KfCeK4uqJzJMfhZ",
		"/ip4/3.99.166.147/tcp/16593/p2p/12D3KooWGYUf93iYWsUibSvKdxsYUY1p7fC1nQotCpUcDXD1ABvR",
		"/ip4/16.171.4.135/tcp/16593/p2p/12D3KooWEMpAxKnXJPkcEXpDmrnjrZ5iFMZvvQtimmTTxuoRGkXV",
		"/ip4/54.237.244.42/tcp/16593/p2p/12D3KooWLoMkrgW862Gs152jLt6FiZZs4GkY24Su4QojnvMoSNaQ",
		// "/ip4/3.238.124.43/tcp/16593/p2p/12D3KooWJ8CA8pacTnKWVgBSEav4QG1zJpyeSSME47RugpDUrZp8",
		"/ip4/13.53.125.115/tcp/16593/p2p/12D3KooWBJk52fQExXHWhFNk692hP7JvTxNTvUMdVne8tbJ3DBf3",
		"/ip4/13.59.241.224/tcp/16593/p2p/12D3KooWKjYKqg2TgUSLq8CZAP8G6LhjXUWTcQBd9qYL2JHug9HW",
		"/ip4/18.168.202.86/tcp/16593/p2p/12D3KooWDiKGbUZg1rB5EufRCkRPiDCEPMjyvTfTVR9qsKVVkcuC",
		"/ip4/35.183.112.161/tcp/16593/p2p/12D3KooWFPKeXzKMd3jtoeG6ts6ADKmVV8rVkXR9k9YkQPgpLzd6",
	}
	addrs := make([]multiaddr.Multiaddr, len(s))
	for i, s := range s {
		addr, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			panic(err)
		}
		addrs[i] = addr
	}
	return addrs
}()

func healAnchor(_ *cobra.Command, args []string) {
	ctx, cancel, _ := api.ContextWithBatchData(context.Background())
	defer cancel()

	networkID := args[0]
	node, err := p2p.New(p2p.Options{
		Network: networkID,
		// BootstrapPeers: api.BootstrapServers,
		BootstrapPeers: mainnetAddrs,
	})
	checkf(err, "start p2p node")
	defer func() { _ = node.Close() }()

	fmt.Fprintf(os.Stderr, "We are %v\n", node.ID())

	fmt.Fprintln(os.Stderr, "Waiting for addresses")
	time.Sleep(time.Second)

	// We should be able to use only the p2p client but it doesn't work well for
	// some reason
	///C1 := jsonrpc.NewClient(api.ResolveWellKnownEndpoint(networkID))
	// C1 := jsonrpc.NewClient(api.ResolveWellKnownEndpoint("http://65.109.48.173:16695/v3"))
	C1 := jsonrpc.NewClient(api.ResolveWellKnownEndpoint("http://apollo-mainnet.accumulate.defidevs.io:16695/v3"))
	C1.Client.Timeout = time.Hour

	// Use a hack dialer that uses the API for peer discovery
	router := new(routing.MessageRouter)
	C2 := &message.Client{
		Transport: &message.RoutedTransport{
			Network: networkID,
			Dialer:  node.DialNetwork(),
			// Dialer:  &hackDialer{C1, node.DialNetwork(), map[string]peer.ID{}},
			Router: router,
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
			healSingleAnchor(ctx, C1, C2, net, srcId, dstId, seqNo, nil, map[[32]byte]*protocol.Transaction{})
			return
		}

		ledger := getAccount[*protocol.AnchorLedger](C1, ctx, dstUrl.JoinPath(protocol.AnchorPool))
		healAnchorSequence(ctx, C1, C2, net, srcId, dstId, ledger.Anchor(srcUrl))
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
			healAnchorSequence(ctx, C1, C2, net, src.ID, dst.ID, src2dst)
		}
	}

	// Heal continuously?
	if healContinuous {
		time.Sleep(time.Second)
		goto heal
	}
}

func healAnchorSequence(ctx context.Context, C1 *jsonrpc.Client, C2 *message.Client, net *healing.NetworkInfo, srcId, dstId string, src2dst *protocol.PartitionSyntheticLedger) {
	srcUrl := protocol.PartitionUrl(srcId)
	dstUrl := protocol.PartitionUrl(dstId)

	ids, txns := findPendingAnchors(ctx, C2, api.Querier2{Querier: C1}, srcUrl, dstUrl, true)
	src2dst.Pending = append(src2dst.Pending, ids...)

	for i, txid := range src2dst.Pending {
		healSingleAnchor(ctx, C1, C2, net, srcId, dstId, src2dst.Delivered+1+uint64(i), txid, txns)
	}
}

func healSingleAnchor(ctx context.Context, C1 *jsonrpc.Client, C2 *message.Client, net *healing.NetworkInfo, srcId, dstId string, seqNum uint64, txid *url.TxID, txns map[[32]byte]*protocol.Transaction) {
	srcUrl := protocol.PartitionUrl(srcId)
	dstUrl := protocol.PartitionUrl(dstId)

	if txid == nil {
		// Get a signature from each node that hasn't signed
		for peer, info := range net.Peers[strings.ToLower(srcId)] {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()

			slog.InfoCtx(ctx, "Querying node for its signature", "id", peer)
			res, err := C2.ForPeer(peer).Private().Sequence(ctx, srcUrl.JoinPath(protocol.AnchorPool), dstUrl, seqNum)
			if err != nil {
				slog.ErrorCtx(ctx, "Query failed", "error", err)
				continue
			}

			myTxn, ok := res.Message.(*messaging.TransactionMessage)
			if !ok {
				slog.ErrorCtx(ctx, "Node gave us an anchor that is not a transaction", "id", info, "type", res.Message.Type())
				continue
			}

			txid = myTxn.ID()
			txns[txid.Hash()] = myTxn.Transaction
			break
		}
		// err := healing.HealAnchor(ctx, C1, C2, net, srcUrl, dstUrl, src2dst.Delivered+1+uint64(i), nil, nil, pretend)
		// check(err)
		// continue
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
			return
		}

		seq, ok := res.Message.(*messaging.SequencedMessage)
		if !ok {
			slog.ErrorCtx(ctx, "Message receieved was not a sequenced message")
			return
		}
		txm, ok := seq.Message.(*messaging.TransactionMessage)
		if !ok {
			slog.ErrorCtx(ctx, "Sequenced message does not contain a transaction message")
			return
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
	err = healing.HealAnchor(ctx, C1, C2, net, srcUrl, dstUrl, seqNum, txn, sigSets, pretend)
	check(err)
}

func getAccount[T protocol.Account](C api.Querier, ctx context.Context, u *url.URL) T {
	var v T
	_, err := api.Querier2{Querier: C}.QueryAccountAs(ctx, u, nil, &v)
	checkf(err, "get %v", u)
	return v
}
