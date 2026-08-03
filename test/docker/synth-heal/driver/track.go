// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// maxDepth bounds how far down the produce tree the tracker walks. A user
// transaction produces synthetics (depth 1), which produce receipts and refunds
// (depth 2); three levels covers everything we care about without chasing the
// anchor machinery.
const maxDepth = 3

type count struct{ sent, delivered int }

// root is one submitted message we follow for the verdict, along with the
// workload kind that submitted it.
type root struct {
	kind string
	id   *url.TxID
}

// tracker follows submitted transactions through their produce trees and counts
// how many cross-partition synthetic messages of each type were produced and
// how many of those were delivered.
type tracker struct {
	Q    api.Querier2
	tree *routing.RouteTree

	mu    sync.Mutex
	roots []root
	kinds map[string]*count
	synth map[string]*count
}

func newTracker(q api.Querier2, tree *routing.RouteTree) *tracker {
	return &tracker{Q: q, tree: tree, kinds: map[string]*count{}, synth: map[string]*count{}}
}

// follow records the messages of one submitted envelope. Both the transaction
// and its signature matter: the transaction produces the synthetic transactions
// and the signature produces the CreditPayment and SignatureRequest messages.
func (t *tracker) follow(kind string, ids []*url.TxID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, id := range ids {
		t.roots = append(t.roots, root{kind, id})
	}
}

func (t *tracker) followed() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.roots)
}

func (t *tracker) kindCount(name string) count { return t.get(t.kinds, name) }
func (t *tracker) synthCount(name string) count {
	return t.get(t.synth, name)
}

func (t *tracker) get(m map[string]*count, name string) count {
	t.mu.Lock()
	defer t.mu.Unlock()
	if c, ok := m[name]; ok {
		return *c
	}
	return count{}
}

func (t *tracker) synthNames() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	names := make([]string, 0, len(t.synth))
	for n := range t.synth {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// verify re-walks every tracked transaction until everything is delivered or
// the grace period expires, returning whether the run passed: every tracked
// transaction delivered, every produced synthetic delivered.
//
// It re-walks from scratch each round rather than resuming, because a message
// that was pending last round may be delivered now and the counts must reflect
// the final state, not the union of every intermediate one.
func (t *tracker) verify(ctx context.Context, grace time.Duration) bool {
	deadline := time.Now().Add(grace)
	for {
		pending := t.walkAll(ctx)
		if pending == 0 {
			return true
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			log.Printf("%d messages still undelivered after the grace period", pending)
			return false
		}
		log.Printf("waiting on %d undelivered messages...", pending)
		time.Sleep(15 * time.Second)
	}
}

// walkAll walks every tracked root and returns how many messages are still
// undelivered.
func (t *tracker) walkAll(ctx context.Context) int {
	t.mu.Lock()
	roots := make([]root, len(t.roots))
	copy(roots, t.roots)
	t.kinds = map[string]*count{}
	t.synth = map[string]*count{}
	t.mu.Unlock()

	const workers = 8
	var wg sync.WaitGroup
	var pending struct {
		sync.Mutex
		n int
	}
	ch := make(chan root)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range ch {
				n := t.walk(ctx, r)
				pending.Lock()
				pending.n += n
				pending.Unlock()
			}
		}()
	}
	for _, r := range roots {
		if ctx.Err() != nil {
			break
		}
		ch <- r
	}
	close(ch)
	wg.Wait()
	return pending.n
}

// walk follows one root message and everything it produced, returning the
// number of messages that are not yet delivered.
func (t *tracker) walk(ctx context.Context, r root) int {
	rec, err := t.Q.QueryMessage(ctx, r.id, nil)
	if err != nil || rec == nil {
		// Not indexed yet, or the node is unreachable — count it as pending so
		// the caller retries.
		return 1
	}

	// Only count the transaction itself against the workload kind; the
	// signature that came with it is not a separate unit of work.
	if _, ok := rec.Message.(*messaging.TransactionMessage); ok {
		t.bump(t.kinds, r.kind, rec.Status.Delivered())
	}

	pending := 0
	if !rec.Status.Delivered() {
		pending++
	}
	pending += t.descend(ctx, rec, 0, map[[32]byte]bool{})
	return pending
}

// descend counts the cross-partition messages produced by rec and recurses.
func (t *tracker) descend(ctx context.Context, rec *api.MessageRecord[messaging.Message], depth int, seen map[[32]byte]bool) int {
	if rec.Produced == nil || depth >= maxDepth {
		return 0
	}
	src := t.route(rec.ID)

	pending := 0
	for _, p := range rec.Produced.Records {
		id := p.Value
		if id == nil || seen[id.Hash()] {
			continue
		}
		seen[id.Hash()] = true

		sub, err := t.Q.QueryMessage(ctx, id, nil)
		if err != nil || sub == nil {
			// Produced but not yet visible anywhere: that is exactly the wedge
			// healing is supposed to clear, so keep waiting.
			pending++
			continue
		}

		// Only messages that actually cross a partition boundary are synthetic
		// traffic that healing has to recover; a produce within one partition
		// is executed inline.
		if dst := t.route(id); src != "" && dst != "" && !strings.EqualFold(src, dst) {
			t.bump(t.synth, typeName(sub), sub.Status.Delivered())
			if !sub.Status.Delivered() {
				pending++
			}
		}

		pending += t.descend(ctx, sub, depth+1, seen)
	}
	return pending
}

// route reports which partition a message's account lives on.
func (t *tracker) route(id *url.TxID) string {
	if id == nil {
		return ""
	}
	part, err := t.tree.Route(id.Account())
	if err != nil {
		return ""
	}
	return part
}

// typeName names a message for the coverage report: the transaction body type
// for synthetic transactions, the message type for everything else. Synthetic
// messages arrive wrapped in sequencing envelopes, so unwrap first.
func typeName(rec *api.MessageRecord[messaging.Message]) string {
	var m messaging.Message = rec.Message
	for {
		u, ok := m.(interface{ Unwrap() messaging.Message })
		if !ok {
			break
		}
		m = u.Unwrap()
	}
	if m == nil {
		return "unknown"
	}
	if tm, ok := m.(messaging.MessageWithTransaction); ok && tm.GetTransaction() != nil && tm.GetTransaction().Body != nil {
		return tm.GetTransaction().Body.Type().String()
	}
	return m.Type().String()
}

func (t *tracker) bump(m map[string]*count, name string, delivered bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	c, ok := m[name]
	if !ok {
		c = new(count)
		m[name] = c
	}
	c.sent++
	if delivered {
		c.delivered++
	}
}
