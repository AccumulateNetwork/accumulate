// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
)

type peerQueue struct {
	mu   sync.Mutex
	all  map[peer.ID]*peerQueueNode
	head *peerQueueNode
}

type peerQueueNode struct {
	id peer.ID

	prev, next *peerQueueNode
}

func (q *peerQueue) Has(id peer.ID) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	_, ok := q.all[id]
	return ok
}

func (q *peerQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.all)
}

func (q *peerQueue) All() []peer.ID {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Get the head
	h := q.head
	if h == nil {
		return nil
	}

	// Iterate over the list
	all := make([]peer.ID, 0, len(q.all))
	all = append(all, h.id)
	for n := h.next; n != h; n = n.next {
		all = append(all, n.id)
	}
	return all
}

func (q *peerQueue) Next() (peer.ID, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Get the head
	n := q.head
	if n == nil {
		return "", false
	}

	// Advance the queue
	q.head = n.next
	return n.id, true
}

func (q *peerQueue) Add(id ...peer.ID) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.all == nil {
		q.all = map[peer.ID]*peerQueueNode{}
	}

	// Check for an existing entry
	var added bool
	for _, id := range id {
		_, ok := q.all[id]
		if ok {
			continue
		}

		// Add a new node
		n := new(peerQueueNode)
		n.id = id
		q.all[id] = n
		added = true

		// Is the list empty?
		h := q.head
		if h == nil {
			n.prev, n.next = n, n
			q.head = n
			continue
		}

		// Add the node to the end
		t := h.prev           // The tail is the node before the head
		t.next, n.prev = n, t // Attach N after T
		n.next, h.prev = h, n // Attach N before H
	}
	return added
}

func (q *peerQueue) Remove(id ...peer.ID) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	var removed bool
	for _, id := range id {
		// Check for an entry
		n, ok := q.all[id]
		if !ok {
			continue
		}

		// Remove the node
		delete(q.all, id)
		removed = true

		// If N is the only node, clear the head
		h := q.head
		if n == h && n.next == n && n.prev == n {
			q.head = nil
			continue
		}

		// If N is the head, set the head to the next node
		if n == h {
			q.head = n.next
		}

		// Unlink N
		n.prev.next = n.next
		n.next.prev = n.prev
	}
	return removed
}
