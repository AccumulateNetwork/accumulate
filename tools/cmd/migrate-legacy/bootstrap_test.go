// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

// A real libp2p peer ID (ed25519, identity multihash).
const realPeerID = "12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg"

// A CometBFT node's persistent peers name it by its CometBFT node ID, which is
// not a libp2p identity: no libp2p node can be dialed by it. The migration
// writes none of them; partition bootstrap peers come from --bootstrap or from
// a running node's API v3 node info (--peer), and the output says when there
// are none (#4490).
func TestMigrateWritesNoFabricatedPeers(t *testing.T) {
	persistent := `priv_validator_key_file = "../priv_validator_key.json"

[p2p]
persistent_peers = "6e56f4363839e9c6a7d24a5030fd78eca6565bec@fozzie-bvn0.accumulate.defidevs.io:16691"
`
	legacyNode := func(t *testing.T) string {
		work := t.TempDir()
		for _, p := range []struct{ dir, acc string }{{"bvnn", minimalBVN}, {"dnn", minimalDN}} {
			writeLegacyPartition(t, work, p.dir, p.acc, "node_key.json")
			require.NoError(t, os.WriteFile(filepath.Join(work, p.dir, "config", "tendermint.toml"), []byte(persistent), 0600))
		}
		return work
	}
	migrated := func(t *testing.T, work string) (*run.CoreValidatorConfiguration, string) {
		cfg := new(run.Config)
		require.NoError(t, cfg.LoadFrom(filepath.Join(work, "accumulate.toml")))
		b, err := os.ReadFile(filepath.Join(work, "accumulate.toml"))
		require.NoError(t, err)
		return cfg.Configurations[0].(*run.CoreValidatorConfiguration), string(b)
	}
	t.Cleanup(func() { flagMigrate.Bootstrap, flagMigrate.Peer = nil, "" })

	t.Run("None", func(t *testing.T) {
		work := legacyNode(t)
		flagMain.WorkDir = work
		flagMigrate.Bootstrap, flagMigrate.Peer = nil, ""
		stderr := captureStderr(t, func() { migrate(nil, nil) })

		cvc, text := migrated(t, work)
		require.Empty(t, cvc.DnBootstrapPeers)
		require.Empty(t, cvc.BvnBootstrapPeers)
		require.NotContains(t, text, "fozzie-bvn0", "a CometBFT persistent peer was carried over")
		require.Contains(t, stderr, "No partition bootstrap peers")
	})

	t.Run("Bootstrap", func(t *testing.T) {
		work := legacyNode(t)
		flagMain.WorkDir = work
		want := "/dns/acc-val1/tcp/16593/p2p/" + realPeerID
		flagMigrate.Bootstrap, flagMigrate.Peer = []string{want}, ""
		stderr := captureStderr(t, func() { migrate(nil, nil) })

		cvc, text := migrated(t, work)
		require.Equal(t, []string{want}, addrStrings(cvc.DnBootstrapPeers))
		require.Equal(t, []string{want}, addrStrings(cvc.BvnBootstrapPeers))
		require.NotContains(t, text, "fozzie-bvn0")
		require.NotContains(t, stderr, "No partition bootstrap peers")
	})

	t.Run("Peer", func(t *testing.T) {
		base := startNodeInfo(t, realPeerID)
		work := legacyNode(t)
		flagMain.WorkDir = work
		flagMigrate.Bootstrap, flagMigrate.Peer = nil, fmt.Sprintf("tcp://127.0.0.1:%d", base)
		captureStderr(t, func() { migrate(nil, nil) })

		cvc, _ := migrated(t, work)
		want := fmt.Sprintf("/ip4/127.0.0.1/tcp/%d/p2p/%s", base+int(config.PortOffsetAccumulateP2P), realPeerID)
		require.Equal(t, []string{want}, addrStrings(cvc.DnBootstrapPeers))
		require.Equal(t, []string{want}, addrStrings(cvc.BvnBootstrapPeers))
	})

	// Main's `accumulated migrate` already wrote such peers into the
	// configurations it converted; converting that shape drops them too.
	t.Run("MainMigrated", func(t *testing.T) {
		dir := t.TempDir()
		writeCometKey(t, dir, "priv_validator_key.json")
		fabricated := "/dns/fozzie-bvn0.accumulate.defidevs.io/tcp/16591/p2p/QmTaoxQm8iW3kunoJzQvDNUmeKBqz8khYKCbR38cqMb3Dq"
		real := "/dns/acc-val1/tcp/16593/p2p/" + realPeerID
		file := filepath.Join(dir, "accumulate.toml")
		require.NoError(t, os.WriteFile(file, []byte(`network = "MainNet"

[p2p]
bootstrap-peers = ["`+real+`"]

[p2p.key]
type = "raw"
address = "`+keyAddr("node_key.json")+`"

[[configurations]]
type = "coreValidator"
mode = "dn"
listen = "/tcp/16591"
dn-bootstrap-peers = ["`+fabricated+`", "`+real+`"]

[configurations.validator-key]
type = "cometPrivValFile"
path = "priv_validator_key.json"
`), 0600))
		flagMain.WorkDir = dir
		captureStderr(t, func() { migrate(nil, nil) })

		cfg := new(run.Config)
		require.NoError(t, cfg.LoadFrom(file))
		cvc := cfg.Configurations[0].(*run.CoreValidatorConfiguration)
		require.Equal(t, []string{real}, addrStrings(cvc.DnBootstrapPeers))
		require.Equal(t, []string{real}, addrStrings(cfg.P2P.BootstrapPeers))
	})
}

func addrStrings(addrs []multiaddr.Multiaddr) []string {
	var s []string
	for _, a := range addrs {
		s = append(s, a.String())
	}
	return s
}

// captureStderr returns what fn writes to stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stderr
	os.Stderr = w
	done := make(chan string)
	go func() { b, _ := io.ReadAll(r); done <- string(b) }()
	defer func() { os.Stderr = orig }()
	fn()
	require.NoError(t, w.Close())
	s := <-done
	_, _ = orig.WriteString(s)
	return s
}

// startNodeInfo serves API v3 node-info, as a running node does at its base
// port plus the API offset, reporting the given peer ID; it returns the base.
func startNodeInfo(t *testing.T, id string) int {
	t.Helper()
	pid, err := peer.Decode(id)
	require.NoError(t, err)
	h, err := jsonrpc.NewHandler(jsonrpc.NodeService{NodeService: nodeInfoOnly{&v3.NodeInfo{PeerID: pid, Network: "Fozzie"}}})
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle("/v3", h)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	return l.Addr().(*net.TCPAddr).Port - int(config.PortOffsetAccumulateApi)
}

type nodeInfoOnly struct{ ni *v3.NodeInfo }

func (n nodeInfoOnly) NodeInfo(context.Context, v3.NodeInfoOptions) (*v3.NodeInfo, error) {
	return n.ni, nil
}

func (nodeInfoOnly) FindService(context.Context, v3.FindServiceOptions) ([]*v3.FindServiceResult, error) {
	return nil, nil
}
