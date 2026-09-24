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
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
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
// next on a transport error OR a NotReady answer, so a single
// paused/restarted/OOM'd/joining node neither fails queries nor concentrates
// load. Business errors pass straight through.
//
// NotReady is the protocol's "ask someone else": a joining node refuses every
// read until it has executed the state it would answer from (executor.md,
// "Sync" step 6; `... is joining and cannot answer for state it has not
// executed`). Returning it to the caller as a failed read, while eleven other
// nodes could have answered, lost 52 reads on run 20260924T052134Z (#4404) —
// the rule #4387 asks of the healing requester, applied here.
//
// Both kinds of hand-off are COUNTED, because a read retried elsewhere is a
// fact about the network the run should state, and a read every endpoint
// refused is a loss the run must not hide:
//
//	notReadyRetried    NotReady answers that were retried at another endpoint (answers)
//	notReadyExhausted  queries every endpoint answered NotReady, returned to the caller (queries)
//	netErrRetried      transport errors that were retried at another endpoint (answers)
//	netErrExhausted    queries every endpoint failed at the transport (queries)
type poolQuerier struct {
	clients []api.Querier
	idx     *atomic.Uint64
	counts  queryCounts
}

// queryCounts are the hand-offs poolQuerier made, for loadgen-stats.json.
type queryCounts struct {
	notReadyRetried   atomic.Uint64
	notReadyExhausted atomic.Uint64
	netErrRetried     atomic.Uint64
	netErrExhausted   atomic.Uint64
}

// queryStats is queryCounts as loadgen-stats.json carries it.
type queryStats struct {
	NotReadyRetried   uint64 `json:"notReadyRetriedElsewhere"`
	NotReadyExhausted uint64 `json:"notReadyAtEveryEndpoint"`
	NetErrRetried     uint64 `json:"transportErrorRetriedElsewhere"`
	NetErrExhausted   uint64 `json:"transportErrorAtEveryEndpoint"`
}

func (c *queryCounts) snapshot() queryStats {
	return queryStats{
		NotReadyRetried:   c.notReadyRetried.Load(),
		NotReadyExhausted: c.notReadyExhausted.Load(),
		NetErrRetried:     c.netErrRetried.Load(),
		NetErrExhausted:   c.netErrExhausted.Load(),
	}
}

// isNotReady reports whether a node answered "not now, ask another": the
// NotReady status, however many wrappers the JSON-RPC client put around it
// (it arrives as UnknownError "request failed: %w" with the node's error as
// the cause).
func isNotReady(err error) bool {
	return err != nil && errors.Is(err, errors.NotReady)
}

func (p *poolQuerier) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	n := len(p.clients)
	start := int(p.idx.Add(1))
	var err error
	for i := 0; i < n; i++ {
		var r api.Record
		r, err = p.clients[(start+i)%n].Query(ctx, scope, query)
		notReady := isNotReady(err)
		if err == nil || (!notReady && !isNetErr(err)) {
			return r, err
		}
		last := i == n-1
		switch {
		case notReady && last:
			p.counts.notReadyExhausted.Add(1)
		case notReady:
			p.counts.notReadyRetried.Add(1)
		case last:
			p.counts.netErrExhausted.Add(1)
		default:
			p.counts.netErrRetried.Add(1)
		}
	}
	return nil, err
}
