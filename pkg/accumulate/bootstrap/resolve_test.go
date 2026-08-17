// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bootstrap

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
)

func mustAddrs(t *testing.T, s ...string) []multiaddr.Multiaddr {
	t.Helper()
	out := make([]multiaddr.Multiaddr, len(s))
	for i, v := range s {
		a, err := multiaddr.NewMultiaddr(v)
		require.NoError(t, err)
		out[i] = a
	}
	return out
}

func strs(a []multiaddr.Multiaddr) []string {
	out := make([]string, len(a))
	for i, v := range a {
		out[i] = v.String()
	}
	return out
}

// serve stands up a fake endpoint and points a well-known network name at it,
// restoring the map afterwards.
func serve(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	ResetCache() // resolutions are cached process-wide; tests must not share them
	t.Cleanup(ResetCache)

	const name = "testonly"
	old, had := accumulate.WellKnownNetworks[name]
	accumulate.WellKnownNetworks[name] = srv.URL
	t.Cleanup(func() {
		if had {
			accumulate.WellKnownNetworks[name] = old
		} else {
			delete(accumulate.WellKnownNetworks, name)
		}
	})
	return name
}

func respond(t *testing.T, w http.ResponseWriter, results []map[string]any) {
	t.Helper()
	require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0", "id": 1, "result": results,
	}))
}

var fallback = []string{"/dns/bootstrap.accumulatenetwork.io/tcp/16593/p2p/12D3KooWQaWn1L63nJUxfidDomh6W6o1jXJ1VHykzEEdKASSbURr"}

func TestResolve_JoinsPeerIDToAddresses(t *testing.T) {
	name := serve(t, func(w http.ResponseWriter, r *http.Request) {
		respond(t, w, []map[string]any{{
			"peerID":    "12D3KooWMkspfWTgpHGAmDvcCk9CJgeqDgWXcayfNbmvpu5AXKQL",
			"addresses": []string{"/ip4/206.191.154.166/tcp/16593"},
		}})
	})

	got := Resolve(context.Background(), name, mustAddrs(t, fallback...))

	// The API returns peer ID and transport address separately. libp2p
	// authenticates the peer ID and will not dial without it, so the two must
	// be joined or the result is unusable.
	require.Equal(t, []string{
		"/ip4/206.191.154.166/tcp/16593/p2p/12D3KooWMkspfWTgpHGAmDvcCk9CJgeqDgWXcayfNbmvpu5AXKQL",
	}, strs(got))
}

// Each of these must yield the fallback. A node has to start even when
// discovery is broken; this package can improve bootstrapping but must never
// be able to prevent it.
func TestResolve_FallsBackOnEveryFailure(t *testing.T) {
	fb := mustAddrs(t, fallback...)

	t.Run("unknown network attempts no HTTP at all", func(t *testing.T) {
		var called bool
		serve(t, func(w http.ResponseWriter, r *http.Request) { called = true })
		got := Resolve(context.Background(), "DevNet", fb)
		require.Equal(t, strs(fb), strs(got))
		require.False(t, called, "a non-well-known network must not make a request")
	})

	t.Run("http error", func(t *testing.T) {
		name := serve(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		require.Equal(t, strs(fb), strs(Resolve(context.Background(), name, fb)))
	})

	t.Run("rpc error", func(t *testing.T) {
		name := serve(t, func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": 1,
				"error": map[string]any{"message": "invalid Service Type"},
			})
		})
		require.Equal(t, strs(fb), strs(Resolve(context.Background(), name, fb)))
	})

	t.Run("malformed body", func(t *testing.T) {
		name := serve(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("this is not json"))
		})
		require.Equal(t, strs(fb), strs(Resolve(context.Background(), name, fb)))
	})

	t.Run("no peers", func(t *testing.T) {
		name := serve(t, func(w http.ResponseWriter, r *http.Request) { respond(t, w, nil) })
		require.Equal(t, strs(fb), strs(Resolve(context.Background(), name, fb)))
	})

	t.Run("every peer unroutable", func(t *testing.T) {
		name := serve(t, func(w http.ResponseWriter, r *http.Request) {
			respond(t, w, []map[string]any{{
				"peerID":    "12D3KooWMkspfWTgpHGAmDvcCk9CJgeqDgWXcayfNbmvpu5AXKQL",
				"addresses": []string{"/ip4/127.0.0.1/tcp/16593"},
			}})
		})
		require.Equal(t, strs(fb), strs(Resolve(context.Background(), name, fb)))
	})

	t.Run("endpoint hangs past the timeout", func(t *testing.T) {
		name := serve(t, func(w http.ResponseWriter, r *http.Request) {
			// Bounded, not indefinite. A handler that waits only on
			// r.Context() can block forever: the client's cancellation does
			// not always surface as a server-side disconnect, and
			// httptest.Server.Close then waits on the handler that is waiting
			// on the client. The timer guarantees the handler returns.
			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
			}
		})
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		start := time.Now()
		require.Equal(t, strs(fb), strs(Resolve(ctx, name, fb)))
		require.Less(t, time.Since(start), DefaultTimeout, "must honour the caller's deadline")
	})
}

// The deployed API may predate #4091 and still advertise loopback. A client
// cannot assume the node it asks has been upgraded, so it filters too.
func TestResolve_DropsUnroutableAddresses(t *testing.T) {
	name := serve(t, func(w http.ResponseWriter, r *http.Request) {
		respond(t, w, []map[string]any{{
			"peerID": "12D3KooWMkspfWTgpHGAmDvcCk9CJgeqDgWXcayfNbmvpu5AXKQL",
			"addresses": []string{
				"/ip4/127.0.0.1/tcp/16593",
				"/ip4/10.0.0.4/tcp/16593",
				"/ip4/206.191.154.166/tcp/16593",
			},
		}})
	})

	got := Resolve(context.Background(), name, mustAddrs(t, fallback...))
	require.Equal(t, []string{
		"/ip4/206.191.154.166/tcp/16593/p2p/12D3KooWMkspfWTgpHGAmDvcCk9CJgeqDgWXcayfNbmvpu5AXKQL",
	}, strs(got))
}

func TestResolve_CapsAndDeduplicates(t *testing.T) {
	name := serve(t, func(w http.ResponseWriter, r *http.Request) {
		var results []map[string]any
		for i := 0; i < MaxPeers*2; i++ {
			results = append(results, map[string]any{
				"peerID": "12D3KooWMkspfWTgpHGAmDvcCk9CJgeqDgWXcayfNbmvpu5AXKQL",
				// Same address repeated: a hostile or broken endpoint must not
				// be able to inflate the list.
				"addresses": []string{"/ip4/206.191.154.166/tcp/16593", "/ip4/206.191.154.166/tcp/16593"},
			})
		}
		respond(t, w, results)
	})

	got := Resolve(context.Background(), name, mustAddrs(t, fallback...))
	require.Len(t, got, 1, "identical entries collapse to one")
}

func TestResolve_RequestShape(t *testing.T) {
	var body map[string]any
	name := serve(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		respond(t, w, nil)
	})

	Resolve(context.Background(), name, nil)

	require.Equal(t, "find-service", body["method"])
	params := body["params"].(map[string]any)
	require.Equal(t, name, params["network"])
	svc := params["service"].(map[string]any)

	// "node" is the intuitive choice and returns zero peers on mainnet
	// (#4065). Directory exists on every Accumulate network and its servers
	// answer, so that is what is asked for.
	require.Equal(t, "query", svc["type"])
	require.Equal(t, "Directory", svc["argument"])
}

func TestEndpointFor(t *testing.T) {
	// An unknown name must not be treated as a URL. ResolveWellKnownEndpoint
	// returns its input unchanged for unknown names, which would have this
	// package POST to a garbage address.
	_, ok := endpointFor("DevNet")
	require.False(t, ok)

	got, ok := endpointFor("MainNet") // case-insensitive
	require.True(t, ok)
	require.Equal(t, accumulate.MainNetEndpoint+"/v3", got)
}

func TestAugment(t *testing.T) {
	pinned := mustAddrs(t,
		"/ip4/198.51.100.9/tcp/16593/p2p/12D3KooWCE9AYGgNmuk8Ss3vDgLnPnkyLbNRDcbFvTrhVEQhL5xL")

	t.Run("keeps configured peers first and adds discovered ones", func(t *testing.T) {
		name := serve(t, func(w http.ResponseWriter, r *http.Request) {
			respond(t, w, []map[string]any{{
				"peerID":    "12D3KooWMkspfWTgpHGAmDvcCk9CJgeqDgWXcayfNbmvpu5AXKQL",
				"addresses": []string{"/ip4/206.191.154.166/tcp/16593"},
			}})
		})

		got := Augment(context.Background(), name, pinned)

		// Order matters: an operator's pin is tried before anything the
		// network volunteered. Replacing the list would override a
		// deliberate choice, which is what this function exists to avoid.
		require.Equal(t, []string{
			"/ip4/198.51.100.9/tcp/16593/p2p/12D3KooWCE9AYGgNmuk8Ss3vDgLnPnkyLbNRDcbFvTrhVEQhL5xL",
			"/ip4/206.191.154.166/tcp/16593/p2p/12D3KooWMkspfWTgpHGAmDvcCk9CJgeqDgWXcayfNbmvpu5AXKQL",
		}, strs(got))
	})

	t.Run("configured peers survive a dead endpoint untouched", func(t *testing.T) {
		name := serve(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		})
		require.Equal(t, strs(pinned), strs(Augment(context.Background(), name, pinned)))
	})

	t.Run("does not duplicate a peer that appears in both", func(t *testing.T) {
		shared := "/ip4/206.191.154.166/tcp/16593/p2p/12D3KooWMkspfWTgpHGAmDvcCk9CJgeqDgWXcayfNbmvpu5AXKQL"
		name := serve(t, func(w http.ResponseWriter, r *http.Request) {
			respond(t, w, []map[string]any{{
				"peerID":    "12D3KooWMkspfWTgpHGAmDvcCk9CJgeqDgWXcayfNbmvpu5AXKQL",
				"addresses": []string{"/ip4/206.191.154.166/tcp/16593"},
			}})
		})
		got := Augment(context.Background(), name, mustAddrs(t, shared))
		require.Equal(t, []string{shared}, strs(got))
	})

	t.Run("unknown network is a no-op", func(t *testing.T) {
		require.Equal(t, strs(pinned), strs(Augment(context.Background(), "DevNet", pinned)))
	})
}

// Several nodes are routinely created in one process. Before caching, each
// paid its own round trip — 19 seconds of extra wall clock on the
// cmd/accumulated/run tests.
func TestResolve_CachesAcrossCalls(t *testing.T) {
	var calls int
	name := serve(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		respond(t, w, []map[string]any{{
			"peerID":    "12D3KooWMkspfWTgpHGAmDvcCk9CJgeqDgWXcayfNbmvpu5AXKQL",
			"addresses": []string{"/ip4/206.191.154.166/tcp/16593"},
		}})
	})

	for i := 0; i < 5; i++ {
		require.Len(t, Resolve(context.Background(), name, nil), 1)
	}
	require.Equal(t, 1, calls, "five nodes, one request")
}

// The failure case is the one that matters most: a process that cannot reach
// the endpoint must not pay the timeout again for every node it starts, which
// is exactly when startup is already struggling.
func TestResolve_CachesFailures(t *testing.T) {
	var calls int
	name := serve(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadGateway)
	})

	fb := mustAddrs(t, fallback...)
	for i := 0; i < 5; i++ {
		require.Equal(t, strs(fb), strs(Resolve(context.Background(), name, fb)))
	}
	require.Equal(t, 1, calls, "a failed resolution is not retried per node")
}
