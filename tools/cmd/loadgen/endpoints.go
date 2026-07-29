// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"strings"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// isNetErr reports whether err is a transport/connection failure rather than a
// business rejection (insufficientBalance, unauthorized, ...). A single node
// that chaos paused/restarted (or that OOM'd) produces exactly these, and
// rotating to another endpoint recovers them; a business error must NOT be
// retried, it is a real result.
// hashString is a stable FNV-1a hash, used to pin a signer to an endpoint so a
// signer's ordered transactions always reach the same mempool.
func hashString(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// splitEndpoints parses a comma-separated endpoint list, trimming blanks.
func splitEndpoints(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func isNetErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	for _, p := range []string{
		"connection refused", "connection reset", "EOF",
		"context deadline exceeded", "Client.Timeout",
		"no such host", "i/o timeout", "broken pipe",
		"connect: ", "dial tcp", "server misbehaving",
	} {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

// poolQuerier round-robins queries across all node endpoints, advancing to the
// next on a transport error, so a single paused/restarted/OOM'd node neither
// fails queries nor concentrates load. Business errors pass straight through.
type poolQuerier struct {
	clients []*jsonrpc.Client
	idx     *atomic.Uint64
}

func (p *poolQuerier) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	n := len(p.clients)
	start := int(p.idx.Add(1))
	var err error
	for i := 0; i < n; i++ {
		var r api.Record
		r, err = p.clients[(start+i)%n].Query(ctx, scope, query)
		if err == nil || !isNetErr(err) {
			return r, err
		}
	}
	return nil, err
}
