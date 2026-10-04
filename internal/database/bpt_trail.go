// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"strings"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
)

// BPTWriteSink is told what each commit of a batch begun on the database did
// to the stored blocks below the BPT's root, and who wrote it at which block:
// the write trail a post mortem of a tree that fails its own check reads
// (executor spec, "Sync", §3b).
type BPTWriteSink interface {
	BPTWrites(who string, block uint64, writes []bpt.BlockWrite)
}

// BPTWriteTag is who an outermost batch writes for, and at which block. Every
// batch begun from it shares it.
type BPTWriteTag struct {
	Who   string
	Block uint64
}

// bptSinks holds the database's write sink. It is shared by the database's
// views, and set while batches are being begun on other goroutines.
type bptSinks struct{ v atomic.Pointer[sinkBox] }

type sinkBox struct{ sink BPTWriteSink }

// SetBPTWriteSink has every outermost batch begun on the database from here on
// report its BPT commits to sink. A nil sink stops it.
func (d *Database) SetBPTWriteSink(sink BPTWriteSink) {
	if sink == nil {
		d.sinks.v.Store(nil)
		return
	}
	d.sinks.v.Store(&sinkBox{sink})
}

func (d *Database) bptSink() BPTWriteSink {
	if d.sinks == nil {
		return nil
	}
	if b := d.sinks.v.Load(); b != nil {
		return b.sink
	}
	return nil
}

// TagWrites says who the outermost batch b belongs to writes for, and at
// which block, for the write trail.
func (b *Batch) TagWrites(who string, block uint64) {
	for b.parent != nil {
		b = b.parent
	}
	if b.bptTag != nil {
		b.bptTag.Who, b.bptTag.Block = who, block
	}
}

// AddWriter adds who to the writers of the outermost batch b belongs to: a
// section loaded into a block's batch is written by the block and the load,
// and the trail says both.
func (b *Batch) AddWriter(who string) {
	for b.parent != nil {
		b = b.parent
	}
	t := b.bptTag
	if t == nil {
		return
	}
	for _, w := range strings.Split(t.Who, "+") {
		if w == who {
			return
		}
	}
	if t.Who == "" {
		t.Who = who
	} else {
		t.Who += "+" + who
	}
}

// traceBPT has an outermost batch's tree report its commits to the sink.
func traceBPT(c *Batch, t *bpt.BPT) {
	if c == nil || c.parent != nil || c.bptSink == nil {
		return
	}
	sink, tag := c.bptSink, c.bptTag
	t.SetCommitTrace(func(w []bpt.BlockWrite) {
		var who string
		var block uint64
		if tag != nil {
			who, block = tag.Who, tag.Block
		}
		sink.BPTWrites(who, block, w)
	})
}
