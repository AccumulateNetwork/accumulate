// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"encoding/binary"
	"hash/fnv"
)

// Choosing which worker handles a transaction.
//
// Which worker handles a transaction has to be a function of a routing KEY, and
// for a single signer that key must be stable — otherwise the signer's
// transactions are spread across workers, batched independently and
// committed in whatever order the DAG produces. Replay protection requires a
// signer's timestamps to be strictly increasing IN EXECUTION ORDER, so a
// shuffled signer loses everything but an increasing subsequence: 96 of 100
// transactions were rejected that way, silently, in run 20260822T063 (#4132).
//
// The previous routing was `counter % len(workers)` — a global round-robin
// with no key at all, which spreads one signer's transactions deliberately.
// It breaks ordering at two workers; the worker count only sets the scale.
//
// The worker count is a power of two so the key can be masked rather than
// divided. A mask gives uniform buckets and a stable, cheap mapping; `% 100`
// on a hash gives neither (#4133).

// IsPowerOfTwo reports whether n is a power of two. Zero and negatives are not.
func IsPowerOfTwo(n int) bool {
	return n > 0 && n&(n-1) == 0
}

// workerFor maps a routing key to a worker index.
//
// With a power-of-two worker count this is a mask. Any other count still has
// to work — a deployment is not going to be refused service over it — so it
// falls back to modulo, which is uniform enough but loses the properties the
// mask was chosen for. Config validation is where an odd count should be
// complained about, not here.
func workerFor(key []byte, numWorkers int) int {
	if numWorkers <= 1 {
		return 0
	}
	h := fnv.New64a()
	_, _ = h.Write(key)
	sum := h.Sum64()
	if IsPowerOfTwo(numWorkers) {
		return int(sum & uint64(numWorkers-1))
	}
	return int(sum % uint64(numWorkers))
}

// routingKeyBytes renders a routing key for hashing. Separated so a caller can
// pass a signer URL, an account, or anything else stable for the sender.
func routingKeyBytes(key string) []byte {
	if key == "" {
		return nil
	}
	b := make([]byte, 0, len(key)+8)
	b = append(b, key...)
	// Length-prefix guards against two different keys colliding through
	// concatenation if this is ever composed.
	var n [8]byte
	binary.LittleEndian.PutUint64(n[:], uint64(len(key)))
	return append(b, n[:]...)
}
