// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// quic-retention-probe opens and closes QUIC connections to one libp2p host
// and reports how much of what the connections allocated is still reachable
// afterwards (#4542: 53 MiB of quic-go connection state on a validator with
// at most 99 live connections).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
)

var (
	rounds  = flag.Int("rounds", 200, "connections to open and close")
	newPeer = flag.Bool("new-peer", true, "a new dialing host (and peer ID) for every connection, as a node that leaves and joins again")
	stream  = flag.Bool("stream", true, "open a stream and exchange a message on each connection")
	killed  = flag.Bool("killed", false, "each dialer is another process that exits without closing its connection, as a container that is removed")
	dial    = flag.String("dial", "", "internal: connect to this address, exchange a message, and exit without closing")
	node    = flag.Bool("node", false, "the server is an Accumulate p2p node (pkg/api/v3/p2p), not a bare libp2p host")
	gossip  = flag.Bool("pubsub", false, "both ends run GossipSub and share a topic, as consensus does")
)

const topic = "probe-topic"

// joinTopic runs GossipSub on h, subscribed to the topic.
func joinTopic(ctx context.Context, h host.Host) *pubsub.Topic {
	ps, err := pubsub.NewGossipSub(ctx, h)
	check(err)
	t, err := ps.Join(topic)
	check(err)
	sub, err := t.Subscribe()
	check(err)
	go func() {
		for {
			if _, err := sub.Next(ctx); err != nil {
				return
			}
		}
	}()
	return t
}

const proto = "/probe/1"

func newHost() host.Host {
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/udp/0/quic-v1"))
	check(err)
	return h
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// retained is the bytes still in use that were allocated under a function
// whose name contains what.
func retained(what string) (bytes, objects int64) {
	runtime.GC()
	runtime.GC()
	n, _ := runtime.MemProfile(nil, true)
	recs := make([]runtime.MemProfileRecord, n+200)
	n, ok := runtime.MemProfile(recs, true)
	if !ok {
		return -1, -1
	}
	for _, r := range recs[:n] {
		frames := runtime.CallersFrames(r.Stack())
		for {
			f, more := frames.Next()
			if strings.Contains(f.Function, what) {
				bytes += r.InUseBytes()
				objects += r.InUseObjects()
				break
			}
			if !more {
				break
			}
		}
	}
	return bytes, objects
}

func main() {
	flag.Parse()
	ctx := context.Background()
	if *dial != "" {
		info, err := peer.AddrInfoFromString(*dial)
		check(err)
		h := newHost()
		var t *pubsub.Topic
		if *gossip {
			t = joinTopic(ctx, h)
		}
		check(h.Connect(ctx, *info))
		if t != nil {
			time.Sleep(500 * time.Millisecond)
			_ = t.Publish(ctx, []byte("hello"))
		}
		if s, err := h.NewStream(ctx, info.ID, proto); err == nil {
			_, _ = s.Write([]byte("0123456789abcdef"))
			_, _ = s.Read(make([]byte, 16))
		}
		time.Sleep(300 * time.Millisecond) // identify, pubsub and the DHT have their say
		os.Exit(0)                         // nothing is closed
	}
	runtime.MemProfileRate = 1 // every allocation, so the numbers are exact

	var server host.Host
	if *node {
		addr, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/udp/0/quic-v1")
		check(err)
		n, err := p2p.New(p2p.Options{Network: "probe", Listen: []multiaddr.Multiaddr{addr}})
		check(err)
		defer n.Close()
		server = n.Host()
	} else {
		server = newHost()
		defer server.Close()
	}
	if *gossip {
		joinTopic(ctx, server)
	}
	server.SetStreamHandler(proto, func(s network.Stream) {
		buf := make([]byte, 16)
		_, _ = s.Read(buf)
		_, _ = s.Write(buf)
		_ = s.Close()
	})
	info := peer.AddrInfo{ID: server.ID(), Addrs: server.Addrs()}

	var dialer host.Host
	for i := 0; i < *rounds; i++ {
		if *killed {
			addr := fmt.Sprintf("%v/p2p/%v", server.Addrs()[0], server.ID())
			out, err := exec.Command(os.Args[0], "-dial", addr, fmt.Sprintf("-pubsub=%v", *gossip)).CombinedOutput()
			if err != nil {
				fmt.Fprintf(os.Stderr, "dialer: %v: %s\n", err, out)
				os.Exit(1)
			}
			if i%50 == 49 {
				b, o := retained("quic-go.(*connection).preSetup")
				fmt.Printf("after %4d killed dialers: server holds %d live; preSetup still in use %7.1f KiB in %d objects\n",
					i+1, len(server.Network().Conns()), float64(b)/1024, o)
			}
			continue
		}
		if dialer == nil || *newPeer {
			dialer = newHost()
		}
		check(dialer.Connect(ctx, info))
		if *stream {
			s, err := dialer.NewStream(ctx, server.ID(), proto)
			check(err)
			_, err = s.Write([]byte("0123456789abcdef"))
			check(err)
			buf := make([]byte, 16)
			_, _ = s.Read(buf)
			_ = s.Close()
		}
		if *newPeer {
			check(dialer.Close())
		} else {
			check(dialer.Network().ClosePeer(server.ID()))
		}
		if i%50 == 49 {
			time.Sleep(200 * time.Millisecond)
			b, o := retained("quic-go.(*connection).preSetup")
			fmt.Printf("after %4d connections: server holds %d live; preSetup still in use %7.1f KiB in %d objects\n",
				i+1, len(server.Network().Conns()), float64(b)/1024, o)
		}
	}

	for _, wait := range []time.Duration{2 * time.Second, 10 * time.Second, 35 * time.Second, 60 * time.Second} {
		time.Sleep(wait)
		b, o := retained("quic-go.(*connection).preSetup")
		fmt.Printf("%2.0fs later: server holds %d live; preSetup still in use %7.1f KiB in %d objects\n",
			wait.Seconds(), len(server.Network().Conns()), float64(b)/1024, o)
	}
}
