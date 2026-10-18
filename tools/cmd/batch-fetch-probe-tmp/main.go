package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func main() {
	addr := flag.String("addr", "", "peer multiaddr with /p2p/")
	part := flag.String("partition", "BVN2", "")
	hdr := flag.String("header", "", "authored header file")
	flag.Parse()
	data, err := os.ReadFile(*hdr)
	if err != nil {
		panic(err)
	}
	h, err := types.UnmarshalHeader(data)
	if err != nil {
		panic(err)
	}
	host, err := libp2p.New(libp2p.NoListenAddrs)
	if err != nil {
		panic(err)
	}
	ma, _ := multiaddr.NewMultiaddr(*addr)
	ai, err := peer.AddrInfoFromP2pAddr(ma)
	if err != nil {
		panic(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := host.Connect(ctx, *ai); err != nil {
		fmt.Println("connect:", err)
		return
	}
	ph, _ := gossip.NewProtocolHandler(host, *part, nil, nil)
	for _, e := range h.Payload {
		b, err := ph.FetchBatch(ctx, ai.ID, e.Digest)
		switch {
		case err != nil:
			fmt.Printf("batch %x: err %v\n", e.Digest[:8], err)
		case b == nil:
			fmt.Printf("batch %x: peer answered EMPTY (does not hold it)\n", e.Digest[:8])
		default:
			fmt.Printf("batch %x: got %d txs\n", e.Digest[:8], len(b.Transactions))
		}
	}
	for i, p := range h.Parents {
		if i >= 3 {
			break
		}
		_ = p
	}
}
