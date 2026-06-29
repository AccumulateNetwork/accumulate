// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

func TestHttpServicePeers(t *testing.T) {
	const pid = "12D3KooWR3U4854YvJbpFEcDGodoEGvyVg887j24tjuB387mbLAZ"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/peers/bvn1", r.URL.Path)
		_, _ = w.Write([]byte(`{"partition":"bvn1","count":1,"peers":[` +
			`{"peer_id":"` + pid + `","addresses":["/ip4/1.2.3.4/tcp/16593/p2p/` + pid + `"]}]}`))
	}))
	defer srv.Close()

	peers := httpServicePeers(context.Background(), srv.URL, "bvn1")
	require.Len(t, peers, 1)
	require.Equal(t, pid, peers[0].ID.String())
	require.Len(t, peers[0].Addrs, 1, "the /p2p component is stripped into the AddrInfo ID")
}

func TestHttpServicePeers_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	require.Empty(t, httpServicePeers(context.Background(), srv.URL, "bvn1"))
}

func TestRemoteServiceFallback(t *testing.T) {
	const pid = "12D3KooWR3U4854YvJbpFEcDGodoEGvyVg887j24tjuB387mbLAZ"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/peers/bvn1", r.URL.Path)
		_, _ = w.Write([]byte(`{"peers":[{"peer_id":"` + pid + `","addresses":["/ip4/1.2.3.4/tcp/16593/p2p/` + pid + `"]}]}`))
	}))
	defer srv.Close()

	sa, err := api.ServiceTypeSubmit.AddressFor("bvn1").MultiaddrFor("acc")
	require.NoError(t, err)
	peers := remoteServiceFallback([]string{srv.URL})(context.Background(), sa)
	require.Len(t, peers, 1)
	require.Equal(t, pid, peers[0].ID.String())
}
