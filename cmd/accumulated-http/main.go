// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"log/slog"
	"net/http"
	_ "net/http/pprof" //nolint:gosec
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	. "gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	. "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	cmdutil "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

func main() {
	_ = cmd.Execute()
}

var cmd = &cobra.Command{
	Use:   "accumulate-http <network>",
	Short: "Accumulate HTTP API node",
	Run:   run,
	Args:  cobra.ExactArgs(1),
}

var flag = struct {
	Key          PrivateKeyFlag
	LogLevel     string
	HttpListen   []multiaddr.Multiaddr
	PromListen   []multiaddr.Multiaddr
	P2pListen    []multiaddr.Multiaddr
	Peers        []multiaddr.Multiaddr
	Timeout      time.Duration
	ConnLimit    int64
	CorsOrigins  []string
	LetsEncrypt  []string
	TlsCert      string
	TlsKey       string
	PeerDatabase string
	Pprof        string
}{}

var cu = func() *user.User {
	cu, _ := user.Current()
	return cu
}()

func init() {
	flag.Key.Value = &TransientPrivateKey{}

	if cu != nil {
		flag.PeerDatabase = filepath.Join(cu.HomeDir, ".accumulate", "cache", "peerdb.json")
	}

	cmd.Flags().Var(&flag.Key, "key", "The node key - not required but highly recommended. The value can be a key or a file containing a key. The key must be hex, base64, or an Accumulate secret key address.")
	cmd.Flags().VarP((*MultiaddrSliceFlag)(&flag.HttpListen), "http-listen", "l", "HTTP listening address(es) (default /ip4/0.0.0.0/tcp/8080/http)")
	cmd.Flags().Var((*MultiaddrSliceFlag)(&flag.PromListen), "prom-listen", "Prometheus listening address(es)")
	cmd.Flags().Var((*MultiaddrSliceFlag)(&flag.P2pListen), "p2p-listen", "P2P listening address(es)")
	cmd.Flags().VarP((*MultiaddrSliceFlag)(&flag.Peers), "peer", "p", "Peers to connect to")
	cmd.Flags().StringVar(&flag.LogLevel, "log-level", "error", "Log level")
	cmd.Flags().DurationVar(&flag.Timeout, "read-header-timeout", 10*time.Second, "ReadHeaderTimeout to prevent slow loris attacks")
	cmd.Flags().Int64Var(&flag.ConnLimit, "connection-limit", 500, "Limit the number of concurrent connections (set to zero to disable)")
	cmd.Flags().StringSliceVar(&flag.CorsOrigins, "cors-origin", nil, "Allowed CORS origins")
	cmd.Flags().StringSliceVar(&flag.LetsEncrypt, "lets-encrypt", nil, "Enable HTTPS on 443 and use Let's Encrypt to retrieve a certificate. Use of this feature implies acceptance of the LetsEncrypt Terms of Service.")
	cmd.Flags().StringVar(&flag.TlsCert, "tls-cert", "", "Certificate used for HTTPS")
	cmd.Flags().StringVar(&flag.TlsKey, "tls-key", "", "Private key used for HTTPS")
	cmd.Flags().StringVar(&flag.PeerDatabase, "peer-db", flag.PeerDatabase, "Track peers using a persistent database.")
	cmd.Flags().BoolVar(&jsonrpc2.DebugMethodFunc, "debug", false, "Print out a stack trace if an API method fails")
	cmd.Flags().StringVar(&flag.Pprof, "pprof", "", "Address to run net/http/pprof on")

	cmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		if !cmd.Flag("prom-listen").Changed {
			flag.PromListen = []multiaddr.Multiaddr{multiaddr.StringCast("/ip4/0.0.0.0/tcp/8081/http")}
		}
		if !cmd.Flag("peer").Changed {
			flag.Peers = accumulate.BootstrapServers
		}
	}
}

func run(cmd *cobra.Command, args []string) {
	if flag.Pprof != "" {
		s := new(http.Server)
		s.Addr = flag.Pprof
		s.ReadHeaderTimeout = time.Minute
		go func() { Check(s.ListenAndServe()) }() //nolint:gosec
	}

	if !cmd.Flag("peer-db").Changed && cu != nil {
		err := os.MkdirAll(filepath.Join(cu.HomeDir, ".accumulate", "cache"), 0700)
		Check(err)
	}

	if len(flag.HttpListen) == 0 && len(flag.LetsEncrypt) == 0 {
		// Default listen address
		a, err := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/8080/http")
		Check(err)
		flag.HttpListen = []multiaddr.Multiaddr{a}
	}
	if len(flag.Peers) == 0 {
		Fatalf("must specify at least one peer")
	}

	http := &HttpService{
		HttpListener: HttpListener{
			Listen:            flag.HttpListen,
			ConnectionLimit:   &flag.ConnLimit,
			ReadHeaderTimeout: (*encoding.Duration)(&flag.Timeout),
			TlsCertPath:       flag.TlsCert,
			TlsKeyPath:        flag.TlsKey,
		},
		CorsOrigins: flag.CorsOrigins,
		LetsEncrypt: flag.LetsEncrypt,
		Router:      ServiceValue(&RouterService{}),
	}
	cfg := &Config{
		Network: args[0],
		Logging: &Logging{
			Rules: []*LoggingRule{{
				Level: slog.LevelInfo,
			}},
		},
		Instrumentation: &Instrumentation{
			HttpListener: HttpListener{
				Listen: flag.PromListen,
			},
		},
		P2P: &P2P{
			Key:                flag.Key.Value,
			Listen:             flag.P2pListen,
			BootstrapPeers:     flag.Peers,
			PeerDB:             &flag.PeerDatabase,
			EnablePeerTracking: true,
		},
		Services: []Service{http},
	}

	if strings.EqualFold(args[0], "MainNet") {
		// Hard code the peers used for the MainNet as a hack for stability
		http.PeerMap = []*HttpPeerMapEntry{
			{
				ID:         mustParsePeer("12D3KooWAgrBYpWEXRViTnToNmpCoC3dvHdmR6m1FmyKjDn1NYpj"),
				Addresses:  []multiaddr.Multiaddr{mustParseMulti("/dns/apollo-mainnet.accumulate.defidevs.io")},
				Partitions: []string{"Apollo", "Directory"},
			},
			{
				ID:         mustParsePeer("12D3KooWDqFDwjHEog1bNbxai2dKSaR1aFvq2LAZ2jivSohgoSc7"),
				Addresses:  []multiaddr.Multiaddr{mustParseMulti("/dns/yutu-mainnet.accumulate.defidevs.io")},
				Partitions: []string{"Yutu", "Directory"},
			},
			{
				ID:         mustParsePeer("12D3KooWHzjkoeAqe7L55tAaepCbMbhvNu9v52ayZNVQobdEE1RL"),
				Addresses:  []multiaddr.Multiaddr{mustParseMulti("/dns/chandrayaan-mainnet.accumulate.defidevs.io")},
				Partitions: []string{"Chandrayaan", "Directory"},
			},
		}
	}

	ctx := cmdutil.ContextForMainProcess(context.Background())
	inst, err := Start(ctx, cfg)
	Check(err)
	<-inst.Done()
	inst.Stop()
}

func mustParsePeer(s string) peer.ID {
	id, err := peer.Decode(s)
	if err != nil {
		panic(err)
	}
	return id
}

func mustParseMulti(s string) multiaddr.Multiaddr {
	addr, err := multiaddr.NewMultiaddr(s)
	if err != nil {
		panic(err)
	}
	return addr
}
