// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
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

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
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

	mu    sync.Mutex
	open  map[string]RepairEntry // failures logged and not yet closed, by ID
	trail func(prefix byte) []string
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
	if r.trail != nil && len(b.Prefix) == 1 {
		e.Trail = r.trail(b.Prefix[0])
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
