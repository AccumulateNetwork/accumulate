// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"log/slog"
	"strconv"
	"testing"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// tcpPort extracts the TCP port from a multiaddr.
func tcpPort(t *testing.T, a multiaddr.Multiaddr) int {
	t.Helper()
	v, err := a.ValueForProtocol(multiaddr.P_TCP)
	require.NoError(t, err, "address %v has no TCP port", a)
	p, err := strconv.Atoi(v)
	require.NoError(t, err)
	return p
}

// TestDevnetPartitionsGetDistinctPorts pins that every partition in a devnet
// listens on its own port.
//
// A normal deployment runs one BVN per node, so every BVN could share
// PortOffsetBlockValidator — nodes are separated by address. A devnet runs
// several BVNs in one process on one host, and separating them by address
// alone left BVN1, BVN2 and BVN3 all listening on base+100, differing only by
// loopback address. That makes a partition impossible to address by port:
// nothing outside the host can reach a specific BVN, a firewall or proxy
// cannot publish one, and a follower cannot be pointed at one. Each BVN now
// gets its own offset.
func TestDevnetPartitionsGetDistinctPorts(t *testing.T) {
	const base = 46656
	const bvns = 3

	cfg := &Config{
		Network: "TestDevNetPorts",
		Logging: &Logging{Format: "plain", Rules: []*LoggingRule{{Level: slog.LevelError}}},
		P2P:     &P2P{Key: &PrivateKeySeed{Seed: record.NewKey("test-devnet-ports")}},
	}
	d := &DevnetConfiguration{
		Listen:     multiaddr.StringCast("/tcp/" + strconv.Itoa(base)),
		Bvns:       bvns,
		Validators: 1,
	}
	cfg.Configurations = []Configuration{d}
	cfg.file = t.TempDir() + "/accumulate.toml"

	ctx := logging.With(context.Background(), "test", t.Name())
	inst, err := New(ctx, cfg)
	require.NoError(t, err)
	inst.rootDir = t.TempDir()

	require.NoError(t, d.apply(inst, cfg))

	// Collect the consensus listen port of every partition on every sub-node.
	ports := map[string]map[int]bool{}
	for _, svc := range cfg.Services {
		sub, ok := svc.(*SubnodeService)
		if !ok {
			continue
		}
		for _, ss := range sub.Services {
			cs, ok := ss.(*ConsensusService)
			if !ok {
				continue
			}
			id := cs.App.partition().ID
			if ports[id] == nil {
				ports[id] = map[int]bool{}
			}
			ports[id][tcpPort(t, cs.Listen)] = true
		}
	}

	require.NotEmpty(t, ports, "no consensus services were generated")

	// Every partition must be on exactly one port, and no two partitions may
	// share one — that is what makes a partition individually addressable.
	seen := map[int]string{}
	for id, set := range ports {
		require.Len(t, set, 1, "partition %s listens on more than one port: %v", id, set)
		for p := range set {
			if other, dup := seen[p]; dup {
				t.Errorf("partitions %s and %s both listen on port %d — "+
					"neither can be reached or published individually", other, id, p)
			}
			seen[p] = id
		}
	}

	// And the offsets are the documented ones: directory at the base, BVNn at
	// base + 100n, so a devnet based at 16591 yields the mainnet-shaped
	// 16592/16692/16792/16892 RPC ports.
	want := map[string]int{"Directory": base}
	for i := 1; i <= bvns; i++ {
		want["BVN"+strconv.Itoa(i)] = base + 100*i
	}
	for id, expect := range want {
		set, ok := ports[id]
		require.True(t, ok, "partition %s was not generated (got %v)", id, ports)
		require.True(t, set[expect], "partition %s listens on %v, want %d", id, set, expect)
	}
}
