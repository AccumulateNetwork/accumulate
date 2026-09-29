// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// ProducedKindsEvery is the least block time between two `Produced messages`
// lines. What is produced between them is counted into the next line, so
// nothing produced goes unreported.
const ProducedKindsEvery = 10 * time.Second

// maxReemitCauses bounds how many distinct re-emitting causes one window
// names; past it they are counted, not named.
const maxReemitCauses = 5

// producedLogState accumulates what the blocks produced since the last line.
// Under the executor because it spans blocks; written from the block's
// serial end.
type producedLogState struct {
	mu       sync.Mutex
	at       time.Time // block time of the last line
	from, to uint64    // blocks counted since
	count    int
	kinds    map[string]int // destination + message kind
	causes   map[string]int // cause kind
	reemit   map[[32]byte]*reemitted
	others   int // re-emissions past maxReemitCauses distinct causes
}

// reemitted is one cause that produced again in this window what it had
// already produced before.
type reemitted struct {
	kind   string
	before int // what it had produced before its latest execution
	times  int // messages it produced again in the window
}

// messageKind names a message by its type and what it carries: the body type
// of a transaction, the type of a signature.
func messageKind(msg messaging.Message) string {
	if msg == nil {
		return "?"
	}
	k := msg.Type().String()
	if m, ok := messaging.UnwrapAs[messaging.MessageWithTransaction](msg); ok {
		if txn := m.GetTransaction(); txn != nil && txn.Body != nil {
			return k + "/" + txn.Body.Type().String()
		}
	}
	if m, ok := messaging.UnwrapAs[messaging.MessageWithSignature](msg); ok {
		if sig := m.GetSignature(); sig != nil {
			return k + "/" + sig.Type().String()
		}
	}
	return k
}

// noteProducedKinds counts the messages this block produced for other
// partitions by destination and kind, and their causes by kind, and writes
// the counts as one `Produced messages` line no more often than
// ProducedKindsEvery of block time. A cause that produced again what it had
// produced before is named on a `Cause re-emitted` line with the same
// cadence: its hash, its kind, and how many times (#4465).
//
// dest names the partition each message was sequenced for, in the same order
// as produced; nil counts them by account instead.
func (b *Block) noteProducedKinds(produced []*ProducedMessage, dest []*url.URL) {
	l := &b.Executor.producedLog
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(produced) > 0 {
		if l.kinds == nil {
			l.kinds, l.causes, l.reemit = map[string]int{}, map[string]int{}, map[[32]byte]*reemitted{}
		}
		if l.count == 0 {
			l.from = b.Index
		}
		l.to = b.Index
		for i, p := range produced {
			l.count++
			to := p.Destination
			if i < len(dest) {
				to = dest[i]
			}
			l.kinds[partitionLabel(to)+" "+messageKind(p.Message)]++
			cause := p.cause
			if cause == "" {
				cause = "system"
			}
			l.causes[cause]++
			if p.causeBefore == 0 {
				continue
			}
			r, ok := l.reemit[p.causeHash]
			if !ok && len(l.reemit) >= maxReemitCauses {
				l.others++
				continue
			}
			if !ok {
				r = &reemitted{kind: cause}
				l.reemit[p.causeHash] = r
			}
			r.times++
			if p.causeBefore > r.before {
				r.before = p.causeBefore
			}
		}
	}
	if l.count == 0 || (!l.at.IsZero() && b.Time.Sub(l.at) < ProducedKindsEvery) {
		return
	}

	b.Executor.logger.Info("Produced messages", "module", "stream",
		"blocks", fmt.Sprintf("%d-%d", l.from, l.to), "count", l.count,
		"produced", countsLabel(l.kinds), "causes", countsLabel(l.causes))
	if len(l.reemit) > 0 {
		var named []string
		for h, r := range l.reemit {
			named = append(named, fmt.Sprintf("%s %s before=%d times=%d", hex.EncodeToString(h[:]), r.kind, r.before, r.times))
		}
		sort.Strings(named)
		b.Executor.logger.Info("Cause re-emitted", "module", "stream",
			"blocks", fmt.Sprintf("%d-%d", l.from, l.to), "causes", strings.Join(named, "; "), "others", l.others)
	}
	l.at, l.count, l.others = b.Time, 0, 0
	clear(l.kinds)
	clear(l.causes)
	clear(l.reemit)
}

// countsLabel writes counts as "key=n, ...", largest first.
func countsLabel(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%d", k, m[k])
	}
	return strings.Join(parts, ", ")
}
