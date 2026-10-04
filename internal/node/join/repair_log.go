// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// RepairEntry is one failure the node named and what it did about it, or the
// outcome of that repair (executor spec, "Sync", §3b). The same fields are
// logged at WARN under module=repair and appended to the repair log.
type RepairEntry struct {
	Time      time.Time `json:"time"`
	ID        string    `json:"id"`
	Kind      string    `json:"kind,omitempty"` // section-hash, account-diverged, unlocated
	Partition string    `json:"partition,omitempty"`
	Block     uint64    `json:"block,omitempty"`
	Prefix    string    `json:"prefix,omitempty"`
	Sections  []uint16  `json:"sections,omitempty"`
	Account   string    `json:"account,omitempty"`
	Held      string    `json:"held,omitempty"`
	Expected  string    `json:"expected,omitempty"`
	Peer      string    `json:"peer,omitempty"`
	Action    string    `json:"action,omitempty"`
	Trail     []string  `json:"trail,omitempty"`

	// Outcome is set on the entry that closes a failure: repaired, or failed
	// with Error.
	Outcome string `json:"outcome,omitempty"`
	Error   string `json:"error,omitempty"`
}

// repairs records the failures a node repairs. Path is the repair log, one
// JSON object a line, appended and synced before the repair starts; empty, it
// only logs.
type repairs struct {
	log  *slog.Logger
	path string

	mu     sync.Mutex
	open   map[string]RepairEntry // failures logged and not yet closed, by ID
	trails map[byte][]string      // the write trail of each stored block below the root
}

func newRepairs(log *slog.Logger, path string) *repairs {
	return &repairs{log: log, path: path, open: map[string]RepairEntry{}}
}

func repairID(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// sectionHash records a stored block of the node's own tree that cannot be
// read as the tree's: its sections are condemned and taken again.
func (r *repairs) sectionHash(partition *url.URL, block uint64, b *bpt.InconsistentBlockError, peer *bpt.SectionHashes) {
	var secs []uint16
	if len(b.Prefix) == 1 {
		for n := 0; n < 16; n++ {
			secs = append(secs, uint16(b.Prefix[0])<<4|uint16(n))
		}
	}
	e := RepairEntry{
		Kind:      "section-hash",
		Partition: partition.String(),
		Block:     block,
		Prefix:    hex.EncodeToString(b.Prefix),
		Sections:  secs,
		Held:      hex.EncodeToString(b.SlotsHash[:]),
		Expected:  hex.EncodeToString(b.BlockHash[:]),
		Action:    "condemned the sections under the prefix; they are forgotten, marked not loaded and loaded again",
	}
	if peer != nil {
		root := peer.Root()
		e.Peer = hex.EncodeToString(root[:])
	}
	if len(b.Prefix) == 1 {
		e.Trail = r.trailOf(b.Prefix[0])
	}
	e.ID = repairID(e.Kind, e.Partition, e.Prefix, e.Held, e.Expected)
	r.record(e)
}

// accountsDiverged records a block that left accounts or chains other than
// its record. The record's state, taken from the peer whole, stands for each,
// so the failure is repaired as it is recorded; whether the block's root then
// matches is the root watch's to say.
func (r *repairs) accountsDiverged(partition *url.URL, block uint64, d *divergedBlock) {
	e := RepairEntry{
		Kind:      "account-diverged",
		Partition: partition.String(),
		Block:     block,
		Account:   strings.Join(d.first, " "),
		Held:      fmt.Sprintf("%d differ", d.count),
		Action:    "the record's state, taken from the peer, stands for each",
	}
	e.ID = repairID(e.Kind, e.Partition, fmt.Sprint(block), e.Account)
	r.record(e)
	r.close(e.ID, nil)
}

// repaired closes the failure a stored block was named in.
func (r *repairs) repaired(partition *url.URL, b *bpt.InconsistentBlockError) {
	id := repairID("section-hash", partition.String(), hex.EncodeToString(b.Prefix),
		hex.EncodeToString(b.SlotsHash[:]), hex.EncodeToString(b.BlockHash[:]))
	r.close(id, nil)
}

// record logs a failure and appends it to the repair log before the repair.
func (r *repairs) record(e RepairEntry) {
	e.Time = time.Now().UTC()
	r.mu.Lock()
	_, seen := r.open[e.ID]
	r.open[e.ID] = e
	r.mu.Unlock()
	if seen {
		return // Already logged and not yet closed
	}
	r.log.Warn("A failure this node can name; repairing it", "module", "repair",
		"id", e.ID, "kind", e.Kind, "partition", e.Partition, "block", e.Block,
		"prefix", e.Prefix, "account", e.Account, "held", e.Held, "expected", e.Expected,
		"peer", e.Peer, "action", e.Action, "trail", e.Trail)
	r.append(e)
}

// close records a repair's outcome: repaired when err is nil.
func (r *repairs) close(id string, err error) {
	r.mu.Lock()
	e, ok := r.open[id]
	delete(r.open, id)
	r.mu.Unlock()
	if !ok {
		return
	}
	out := RepairEntry{Time: time.Now().UTC(), ID: id, Outcome: "repaired"}
	if err != nil {
		out.Outcome, out.Error = "failed", err.Error()
		r.log.Warn("A repair failed", "module", "repair", "id", id, "kind", e.Kind, "error", err)
	} else {
		r.log.Info("A failure is repaired", "module", "repair", "id", id, "kind", e.Kind)
	}
	r.append(out)
}

// append writes one entry to the repair log and syncs it. A failure to write
// it is logged and does not stop the repair: the repair log serves an operator,
// not the node.
func (r *repairs) append(e RepairEntry) {
	if r.path == "" {
		return
	}
	err := appendRepair(r.path, e)
	if err != nil {
		r.log.Error("The repair log could not be written", "module", "repair", "path", r.path, "error", err)
	}
}

func appendRepair(path string, e RepairEntry) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("append to %s: %w", path, err)
	}
	return nil
}

// taggedStore begins batches that say who they write for, for the write
// trail.
type taggedStore struct {
	db    *database.Database
	who   string
	block uint64
}

func (t taggedStore) Begin(writable bool) *database.Batch {
	b := t.db.Begin(writable)
	b.TagWrites(t.who, t.block)
	return b
}

// trailDepth is how many writes the trail keeps for each stored block below
// the root.
const trailDepth = 8

// BPTWrites records each commit's writes to the stored blocks below the root,
// keeping the last trailDepth for each: who wrote, at which block, the hash
// the root's block records for it before and after, and whether the block's
// own record was written (executor spec, "Sync", §3b). It implements
// [database.BPTWriteSink].
func (r *repairs) BPTWrites(who string, block uint64, writes []bpt.BlockWrite) {
	if who == "" {
		who = "other"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.trails == nil {
		r.trails = map[byte][]string{}
	}
	for _, w := range writes {
		rec := "not written"
		if w.Written {
			rec = "written"
		}
		line := fmt.Sprintf("%s@%d %x->%x %s", who, block, w.Before[:4], w.After[:4], rec)
		t := append(r.trails[w.Prefix], line)
		if len(t) > trailDepth {
			t = t[len(t)-trailDepth:]
		}
		r.trails[w.Prefix] = t
	}
}

// trailOf is the trail of the stored block under prefix, oldest first.
func (r *repairs) trailOf(prefix byte) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.trails[prefix]...)
}

// readRepairLog reads a repair log, and returns the failures it records that
// no later entry closes.
func readRepairLog(path string) ([]RepairEntry, error) {
	f, err := os.Open(path)
	switch {
	case err == nil:
	case os.IsNotExist(err):
		return nil, nil
	default:
		return nil, err
	}
	defer f.Close()
	var order []string
	open := map[string]RepairEntry{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		var e RepairEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		switch {
		case e.Outcome == "repaired":
			delete(open, e.ID)
		case e.Kind != "":
			if _, ok := open[e.ID]; !ok {
				order = append(order, e.ID)
			}
			open[e.ID] = e
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	var out []RepairEntry
	for _, id := range order {
		if e, ok := open[id]; ok {
			out = append(out, e)
			delete(open, id)
		}
	}
	return out, nil
}

// OfflineRepair is what RepairOffline did.
type OfflineRepair struct {
	Open     int // failures the repair log recorded and no entry had closed
	Repaired int
	Failed   int
}

// RepairOffline repairs, in a stopped node's store, the failures its repair
// log records and nothing has closed (executor spec, "Sync", §3b). A stored
// block of the node's own tree that fails its check is repaired the way a
// running node repairs it: the top of peer's tree is taken, and the sections
// under the block are forgotten and marked not loaded. The next start loads
// them, and its match proves the result. Each failure's outcome is appended to
// the log.
func RepairOffline(ctx context.Context, db *database.Database, partition *url.URL, peer api.Querier, logPath string, log *slog.Logger) (*OfflineRepair, error) {
	if log == nil {
		log = slog.Default()
	}
	open, err := readRepairLog(logPath)
	if err != nil {
		return nil, fmt.Errorf("read the repair log: %w", err)
	}
	out := &OfflineRepair{Open: len(open)}
	if len(open) == 0 {
		return out, nil
	}

	executed, err := readExecutedBlock(db, partition)
	if err != nil {
		return nil, err
	}
	s, err := NewState(StateOptions{Partition: partition, Database: db, Sources: &onePeerSources{partition: partition, peer: peer}, ExecutedBlock: executed, RepairLog: logPath, Logger: log})
	if err != nil {
		return nil, err
	}
	s.sync = newSyncing()
	topErr := s.takeTop(ctx, s.sync)

	batch := db.Begin(false)
	own, err := localTop(batch.BPT())
	batch.Discard()
	if err != nil {
		return nil, fmt.Errorf("read the node's own tree after the repair: %w", err)
	}
	broken := map[string]bool{}
	for _, b := range own.broken {
		broken[hex.EncodeToString(b.Prefix)] = true
	}

	for _, e := range open {
		closed := RepairEntry{Time: time.Now().UTC(), ID: e.ID, Outcome: "repaired"}
		switch {
		case e.Kind != "section-hash":
			closed.Outcome, closed.Error = "failed", "this kind is repaired by the running node, not offline"
		case topErr != nil:
			closed.Outcome, closed.Error = "failed", topErr.Error()
		case broken[e.Prefix]:
			closed.Outcome, closed.Error = "failed", "the block under the prefix still fails its check"
		}
		if closed.Outcome == "repaired" {
			out.Repaired++
		} else {
			out.Failed++
		}
		if err := appendRepair(logPath, closed); err != nil {
			return out, err
		}
	}
	return out, nil
}

// onePeerSources is the one peer an operator names, for every account of the
// partition.
type onePeerSources struct {
	partition *url.URL
	peer      api.Querier
}

func (o *onePeerSources) For(context.Context, *url.URL) ([]pull.Source, *url.URL, error) {
	return []pull.Source{api.Querier2{Querier: o.peer}}, o.partition, nil
}

func (o *onePeerSources) Querier(*url.URL) api.Querier { return o.peer }

func (o *onePeerSources) ValidatorsOf(context.Context, *url.URL) ([]anchorsrc.Validator, error) {
	return nil, errors.NotReady.With("an offline repair reaches no validator")
}
