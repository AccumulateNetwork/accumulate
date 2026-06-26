// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/consensuspeer"
)

type fakeSource struct {
	peers []string
	err   error
}

func (f *fakeSource) ConsensusPeers(context.Context, string) ([]string, error) {
	return f.peers, f.err
}

type fakeDialer struct {
	dialed [][]string
	err    error
}

func (d *fakeDialer) DialPeersAsync(peers []string) error {
	d.dialed = append(d.dialed, peers)
	return d.err
}

func TestFeederRefreshDialsPeers(t *testing.T) {
	src := &fakeSource{peers: []string{"abc@1.2.3.4:16591", "def@5.6.7.8:16591"}}
	dialer := &fakeDialer{}
	f := &ConsensusPeerFeeder{Source: src, Dialer: dialer, Partition: "dn"}

	n, err := f.refreshOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	require.Len(t, dialer.dialed, 1)
	assert.Equal(t, src.peers, dialer.dialed[0])
}

func TestFeederRefreshEmptyDoesNotDial(t *testing.T) {
	dialer := &fakeDialer{}
	f := &ConsensusPeerFeeder{Source: &fakeSource{peers: nil}, Dialer: dialer, Partition: "dn"}

	n, err := f.refreshOnce(context.Background())
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Empty(t, dialer.dialed)
}

func TestFeederRefreshSourceErrorPropagates(t *testing.T) {
	dialer := &fakeDialer{}
	f := &ConsensusPeerFeeder{Source: &fakeSource{err: assert.AnError}, Dialer: dialer, Partition: "dn"}

	_, err := f.refreshOnce(context.Background())
	require.Error(t, err)
	assert.Empty(t, dialer.dialed)
}

func TestFeederRefreshDialErrorSwallowed(t *testing.T) {
	dialer := &fakeDialer{err: assert.AnError}
	f := &ConsensusPeerFeeder{Source: &fakeSource{peers: []string{"abc@1.2.3.4:16591"}}, Dialer: dialer, Partition: "dn"}

	// A dial failure should not bubble up — the loop keeps running.
	n, err := f.refreshOnce(context.Background())
	require.NoError(t, err)
	assert.Zero(t, n, "a dial failure reports zero converged peers")
	require.Len(t, dialer.dialed, 1)
}

func TestHTTPConsensusPeerSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/consensus-peers/dn", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"partition": "dn",
			"count": 2,
			"persistent_peers": "abc@1.2.3.4:16591,def@5.6.7.8:16591",
			"peers": [
				{"node_id":"abc","host":"1.2.3.4","port":16591,"dial":"abc@1.2.3.4:16591"},
				{"node_id":"def","host":"5.6.7.8","port":16591,"dial":"def@5.6.7.8:16591"}
			]
		}`))
	}))
	defer srv.Close()

	src := &HTTPConsensusPeerSource{BaseURL: srv.URL}
	peers, err := src.ConsensusPeers(context.Background(), "dn")
	require.NoError(t, err)
	assert.Equal(t, []string{"abc@1.2.3.4:16591", "def@5.6.7.8:16591"}, peers)
}

func TestHTTPConsensusPeerSourceFallsBackToPersistentPeers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"partition":"dn","count":0,"persistent_peers":"abc@1.2.3.4:16591","peers":[]}`))
	}))
	defer srv.Close()

	src := &HTTPConsensusPeerSource{BaseURL: srv.URL}
	peers, err := src.ConsensusPeers(context.Background(), "dn")
	require.NoError(t, err)
	assert.Equal(t, []string{"abc@1.2.3.4:16591"}, peers)
}

func TestHTTPConsensusPeerSourceStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	src := &HTTPConsensusPeerSource{BaseURL: srv.URL}
	_, err := src.ConsensusPeers(context.Background(), "dn")
	require.Error(t, err)
}

type fakeLister struct{ peers []consensuspeer.Peer }

func (f *fakeLister) GetConsensusPeers(string) []consensuspeer.Peer { return f.peers }

func TestLocalConsensusPeerSource(t *testing.T) {
	src := &LocalConsensusPeerSource{Registry: &fakeLister{peers: []consensuspeer.Peer{
		{ID: "abc", Host: "1.2.3.4", Port: 16591},
		{ID: "def", Host: "5.6.7.8", Port: 16591},
	}}}
	peers, err := src.ConsensusPeers(context.Background(), "dn")
	require.NoError(t, err)
	assert.Equal(t, []string{"abc@1.2.3.4:16591", "def@5.6.7.8:16591"}, peers)
}

func TestMultiConsensusPeerSourceUnionDedup(t *testing.T) {
	m := &MultiConsensusPeerSource{Sources: []ConsensusPeerSource{
		&fakeSource{peers: []string{"a@1.1.1.1:1", "b@2.2.2.2:2"}},
		&fakeSource{peers: []string{"b@2.2.2.2:2", "c@3.3.3.3:3"}}, // b is a duplicate
	}}
	peers, err := m.ConsensusPeers(context.Background(), "dn")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a@1.1.1.1:1", "b@2.2.2.2:2", "c@3.3.3.3:3"}, peers)
}

func TestMultiConsensusPeerSourceFailSoft(t *testing.T) {
	// One source fails; the survivor's peers still come through (no SPOF).
	m := &MultiConsensusPeerSource{Sources: []ConsensusPeerSource{
		&fakeSource{err: assert.AnError},
		&fakeSource{peers: []string{"c@3.3.3.3:3"}},
	}}
	peers, err := m.ConsensusPeers(context.Background(), "dn")
	require.NoError(t, err)
	assert.Equal(t, []string{"c@3.3.3.3:3"}, peers)
}

func TestMultiConsensusPeerSourceAllFail(t *testing.T) {
	m := &MultiConsensusPeerSource{Sources: []ConsensusPeerSource{
		&fakeSource{err: assert.AnError},
		&fakeSource{err: assert.AnError},
	}}
	_, err := m.ConsensusPeers(context.Background(), "dn")
	require.Error(t, err)
}
