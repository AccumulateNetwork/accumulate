// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package peerselect

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

// fakeFinder records calls and returns programmable results.
type fakeFinder struct {
	results map[string][]*api.FindServiceResult
	err     error
	calls   int
}

func (f *fakeFinder) FindService(_ context.Context, opts api.FindServiceOptions) ([]*api.FindServiceResult, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if opts.Service == nil {
		return nil, nil
	}
	return f.results[opts.Service.Argument], nil
}

func TestEligiblePeers_NoneAvailable_ReturnsSentinel(t *testing.T) {
	ff := &fakeFinder{results: map[string][]*api.FindServiceResult{}}
	_, err := EligiblePeers(context.Background(), ff, "testnet", "Directory")
	if !errors.Is(err, ErrNoEligiblePeer) {
		t.Errorf("err=%v, want ErrNoEligiblePeer", err)
	}
}

func TestEligiblePeers_ActivePresent_Returns(t *testing.T) {
	ff := &fakeFinder{
		results: map[string][]*api.FindServiceResult{
			"directory:active": {{PeerID: "peer-A"}},
		},
	}
	got, err := EligiblePeers(context.Background(), ff, "testnet", "Directory")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].PeerID != "peer-A" {
		t.Errorf("got=%+v, want one peer-A", got)
	}
}

func TestEligiblePeers_BothActiveAndComplete_Combined(t *testing.T) {
	ff := &fakeFinder{
		results: map[string][]*api.FindServiceResult{
			"directory:active":   {{PeerID: "peer-A"}},
			"directory:complete": {{PeerID: "peer-B"}, {PeerID: "peer-C"}},
		},
	}
	got, err := EligiblePeers(context.Background(), ff, "testnet", "Directory")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("got %d peers, want 3", len(got))
	}
}

func TestEligiblePeers_FinderError_Bubbles(t *testing.T) {
	ff := &fakeFinder{err: errors.New("transport down")}
	_, err := EligiblePeers(context.Background(), ff, "testnet", "Directory")
	if err == nil || errors.Is(err, ErrNoEligiblePeer) {
		t.Errorf("err=%v, want bubbled transport error", err)
	}
}

func TestEligiblePeers_RejectsMissingInputs(t *testing.T) {
	ff := &fakeFinder{}
	if _, err := EligiblePeers(context.Background(), nil, "n", "p"); err == nil {
		t.Error("expected err for nil finder")
	}
	if _, err := EligiblePeers(context.Background(), ff, "n", ""); err == nil {
		t.Error("expected err for empty partition")
	}
}

func TestPreferAdvertisingPeers(t *testing.T) {
	a := &api.FindServiceResult{PeerID: "advert-1"}
	b := &api.FindServiceResult{PeerID: "advert-2"}
	c := &api.FindServiceResult{PeerID: "legacy-1"}
	d := &api.FindServiceResult{PeerID: "legacy-2"}

	advertSet := map[string]bool{
		"advert-1": true,
		"advert-2": true,
	}
	pred := func(r *api.FindServiceResult) bool { return advertSet[string(r.PeerID)] }

	got := PreferAdvertisingPeers([]*api.FindServiceResult{c, a, d, b}, pred)
	wantOrder := []string{"advert-1", "advert-2", "legacy-1", "legacy-2"}
	for i, w := range wantOrder {
		if string(got[i].PeerID) != w {
			t.Errorf("position %d: got %s, want %s", i, got[i].PeerID, w)
		}
	}
}

func TestPreferAdvertisingPeers_EmptyOrSingle(t *testing.T) {
	if got := PreferAdvertisingPeers(nil, nil); len(got) != 0 {
		t.Errorf("nil input got %d, want 0", len(got))
	}
	one := []*api.FindServiceResult{{PeerID: "x"}}
	if got := PreferAdvertisingPeers(one, nil); len(got) != 1 {
		t.Errorf("single input got %d, want 1", len(got))
	}
}

func TestPartitionFromArgument(t *testing.T) {
	if got := PartitionFromArgument("directory:active"); got != "directory" {
		t.Errorf("got %q, want directory", got)
	}
	if got := PartitionFromArgument("apollo:complete"); got != "apollo" {
		t.Errorf("got %q, want apollo", got)
	}
	if got := PartitionFromArgument("malformed"); got != "" {
		t.Errorf("malformed got %q, want empty", got)
	}
}
