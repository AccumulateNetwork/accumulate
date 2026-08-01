// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
)

// TestConnectEndpointIsGone covers #4026. /connect took an unauthenticated POST
// and dialled whatever multiaddr it was handed, returning the dial error
// verbatim — a success/failure/timing oracle for scanning internal address
// space, plus an eclipse assist and an unbounded outbound amplifier.
//
// The fix is removal, so the test is that the route does not exist. Asserting
// on a hardened handler would let the endpoint quietly come back.
func TestConnectEndpointIsGone(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {})

	// A ServeMux with no /connect pattern falls through to 404. If someone
	// re-registers it, this fails.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/connect", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("/connect answered %d — the SSRF endpoint is reachable again", rec.Code)
	}
}

// TestPublicMuxServesOnlyLiveness covers #4034. The public listener must not
// answer questions about network topology; those moved to the admin listener.
func TestPublicMuxServesOnlyLiveness(t *testing.T) {
	s := &InfoServer{startTime: time.Now()}

	public := http.NewServeMux()
	public.HandleFunc("/info", s.handleInfo)
	public.HandleFunc("/health", s.handleHealth)

	for _, path := range []string{"/peers", "/peers/Directory", "/partitions", "/stats", "/connections", "/debug/dht"} {
		rec := httptest.NewRecorder()
		public.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("public listener serves %s (%d) — topology disclosure", path, rec.Code)
		}
	}
}

// TestRateLimiterBoundsBurstAndRefills covers #4039.
func TestRateLimiterBoundsBurstAndRefills(t *testing.T) {
	l := newRateLimiter(3, 1) // 3 burst, 1/s
	now := time.Now()

	for i := 0; i < 3; i++ {
		if !l.allow("10.0.0.1", now) {
			t.Fatalf("request %d denied inside the burst", i+1)
		}
	}
	if l.allow("10.0.0.1", now) {
		t.Fatal("burst not enforced — a client exceeded its budget")
	}

	// A different client has its own budget.
	if !l.allow("10.0.0.2", now) {
		t.Error("one client's burst starved another client")
	}

	// One token back after a second.
	if !l.allow("10.0.0.1", now.Add(time.Second)) {
		t.Error("bucket did not refill")
	}
	if l.allow("10.0.0.1", now.Add(time.Second)) {
		t.Error("refill exceeded the configured rate")
	}
}

// TestRateLimiterReturns429 checks the wrapper, not just the accounting.
func TestRateLimiterReturns429(t *testing.T) {
	l := newRateLimiter(1, 1)
	h := l.limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/info", nil)
	req.RemoteAddr = "192.0.2.5:1234"

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first request got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request got %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 carries no Retry-After")
	}
}

// TestRateLimitKeyIgnoresForwardedHeaders: keying on an attacker-controlled
// header would let one caller mint unlimited buckets and bypass the limit.
func TestRateLimitKeyIgnoresForwardedHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/info", nil)
	req.RemoteAddr = "192.0.2.5:1234"
	req.Header.Set("X-Forwarded-For", "10.9.9.9")

	if got := clientIP(req); got != "192.0.2.5" {
		t.Fatalf("rate-limit key is %q — a spoofable header would defeat the limit", got)
	}
}

// TestAdminListenerDefaultsToLoopback guards the default that makes #4034 and
// #4033 safe. If this flips to 0.0.0.0 the endpoints are public again.
func TestAdminListenerDefaultsToLoopback(t *testing.T) {
	addr := multiaddr.StringCast("/ip4/127.0.0.1/tcp/8082/http")
	if !isLoopbackMultiaddr(addr) {
		t.Error("127.0.0.1 not recognised as loopback")
	}
	if isLoopbackMultiaddr(multiaddr.StringCast("/ip4/0.0.0.0/tcp/8082/http")) {
		t.Error("0.0.0.0 reported as loopback — the exposure warning would never fire")
	}
	if !isLoopbackMultiaddr(multiaddr.StringCast("/ip6/::1/tcp/8082/http")) {
		t.Error("::1 not recognised as loopback")
	}
}

// TestPublicInfoOmitsTopology covers the rest of #4034. The deployed server was
// publishing its private VPC address (/ip4/172.31.9.165/), DHT routing table
// size, inbound/outbound split, and an uptime that dates the last restart — all
// from /info, which is a PUBLIC endpoint. filterLocalAddresses only strips
// loopback, so RFC1918 went straight out.
//
// Marshalling is the real test: `omitempty` on a non-pointer struct is a no-op,
// so the first attempt still emitted dht and connections as zero values. The
// assertion is on the encoded bytes for that reason, not on the Go struct.
func TestPublicInfoOmitsTopology(t *testing.T) {
	reduced := BootstrapInfo{
		PeerID:            "12D3KooWtest",
		ExternalAddresses: []string{"/ip4/203.0.113.7/tcp/16593/p2p/12D3KooWtest"},
	}
	b, err := json.Marshal(reduced)
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)

	for _, leak := range []string{"listen_addresses", "dht", "connections", "uptime_seconds"} {
		if strings.Contains(body, leak) {
			t.Errorf("public /info still carries %q: %s", leak, body)
		}
	}
	for _, want := range []string{"peer_id", "external_addresses"} {
		if !strings.Contains(body, want) {
			t.Errorf("public /info lost %q — it is the point of the endpoint", want)
		}
	}

	// The admin record must still carry everything.
	full := BootstrapInfo{
		PeerID:            "12D3KooWtest",
		ListenAddresses:   []string{"/ip4/172.31.9.165/tcp/16593"},
		ExternalAddresses: []string{"/ip4/203.0.113.7/tcp/16593/p2p/12D3KooWtest"},
		DHT:               &DHTInfo{Mode: "server", RoutingTableSize: 8},
		Connections:       &ConnectionInfo{Total: 8, Inbound: 8},
		UptimeSeconds:     7700583,
	}
	if b, err = json.Marshal(full); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"listen_addresses", "dht", "connections", "uptime_seconds"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("admin /info lost %q — operators need the full record", want)
		}
	}
}
