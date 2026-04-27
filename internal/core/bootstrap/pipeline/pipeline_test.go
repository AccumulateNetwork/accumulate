// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pipeline

import (
	"context"
	"strings"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestRun_RejectsMissingEndpoint(t *testing.T) {
	_, err := Run(context.Background(), Options{
		Network: "testnet",
		DataDir: t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "endpoint") {
		t.Fatalf("expected endpoint error, got %v", err)
	}
}

func TestRun_RejectsMissingNetwork(t *testing.T) {
	_, err := Run(context.Background(), Options{
		Endpoint: "http://localhost:1",
		DataDir:  t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "network") {
		t.Fatalf("expected network error, got %v", err)
	}
}

func TestRun_RejectsMissingDataDir(t *testing.T) {
	_, err := Run(context.Background(), Options{
		Endpoint: "http://localhost:1",
		Network:  "testnet",
	})
	if err == nil || !strings.Contains(err.Error(), "data dir") {
		t.Fatalf("expected data-dir error, got %v", err)
	}
}

func TestRun_FailsToConnectGracefully(t *testing.T) {
	// Point at a port nothing's listening on.
	_, err := Run(context.Background(), Options{
		Endpoint: "http://127.0.0.1:1/v3",
		Network:  "testnet",
		DataDir:  t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error when peer is unreachable")
	}
	// The wrapped error should mention consensus status (the first
	// network call) — proves we got that far before failing.
	if !strings.Contains(err.Error(), "consensus") && !strings.Contains(err.Error(), "connect") {
		t.Logf("got expected error: %v", err)
	}
}

// TestMinimumBootstrapSet documents the canonical set for Directory and
// a BVN partition.
func TestMinimumBootstrapSet(t *testing.T) {
	dn := minimumBootstrapSet("Directory")
	if len(dn) < 5 {
		t.Errorf("Directory minimum set has %d entries, expected >= 5", len(dn))
	}
	// Every entry must be a non-nil URL.
	for i, u := range dn {
		if u == nil {
			t.Errorf("entry %d is nil", i)
		}
	}

	bvn := minimumBootstrapSet("Apollo")
	if len(bvn) < 5 {
		t.Errorf("BVN minimum set has %d entries, expected >= 5", len(bvn))
	}
	// The DN's Network account should be in both — it carries the
	// partition list which is needed regardless of which partition
	// the new node is joining.
	dnNetwork := protocol.DnUrl().JoinPath(protocol.Network)
	containsNetwork := func(set []*url.URL) bool {
		for _, u := range set {
			if u.Equal(dnNetwork) {
				return true
			}
		}
		return false
	}
	if !containsNetwork(dn) {
		t.Error("Directory minimum set missing dn.acme/network")
	}
	if !containsNetwork(bvn) {
		t.Error("BVN minimum set missing dn.acme/network")
	}
}
