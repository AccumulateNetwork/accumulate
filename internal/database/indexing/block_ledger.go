// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"context"
	"fmt"
	"log/slog"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type BlockLedgerIndexer struct {
	db       database.Beginner
	context  context.Context
	cancel   context.CancelFunc
	url      *url.URL
	scanDone chan struct{}
	read     chan uint64
	write    chan func(func(*database.Batch))
}

func NewBlockLedgerIndexer(ctx context.Context, db database.Beginner, partitionID string) *BlockLedgerIndexer {
	ctx, cancel := context.WithCancel(ctx)
	u := protocol.PartitionUrl(partitionID).JoinPath(protocol.Ledger)
	scanDone := make(chan struct{})
	read := make(chan uint64)
	write := make(chan func(func(*database.Batch)))
	b := &BlockLedgerIndexer{db, ctx, cancel, u, scanDone, read, write}
	go b.scan()
	go b.run()
	return b
}

func (b *BlockLedgerIndexer) Write(batch *database.Batch) {
	done := make(chan struct{})
	select {
	case <-b.context.Done():
	case b.write <- func(f func(*database.Batch)) { f(batch); close(done) }:
		<-done
	}
}

// ScanDone returns a channel that is closed once the block ledger has been
// scanned.
func (b *BlockLedgerIndexer) ScanDone() <-chan struct{} {
	return b.scanDone
}

func (b *BlockLedgerIndexer) scan() {
	defer close(b.scanDone)
	batch := b.db.Begin(false)
	defer batch.Discard()

	for e := range batch.Account(b.url).BlockLedger().Scan().All() {
		select {
		default:
		case <-b.context.Done():
			return
		}

		k, l, err := e.Get()
		if err != nil {
			slog.ErrorContext(b.context, "Error scanning block ledger", "error", err)
			continue
		}

		// Is the entry populated?
		if l.Index != 0 {
			continue
		}

		select {
		case b.read <- k.Get(0).(uint64):
		case <-b.context.Done():
			return
		}
	}
}

func (b *BlockLedgerIndexer) run() {
	batch := b.db.Begin(false)

	// This is a separate closure that captures batch so that it will behave
	// unsurprisingly when the variable is reassigned
	defer func() { batch.Discard() }()

	var entries []*database.BlockLedger
	for {
		select {
		case index := <-b.read:
			var l *protocol.BlockLedger
			err := batch.Account(b.url.JoinPath(fmt.Sprint(index))).Main().GetAs(&l)
			if err != nil {
				slog.ErrorContext(b.context, "Error scanning block ledger", "error", err)
				continue
			}

			entries = append(entries, &database.BlockLedger{
				Index:   l.Index,
				Time:    l.Time,
				Entries: l.Entries,
			})

		case fn := <-b.write:
			if len(entries) == 0 {
				fn(func(batch *database.Batch) {})
				continue
			}

			fn(func(batch *database.Batch) {
				// Write entries
				ledger := batch.Account(b.url).BlockLedger()
				for _, e := range entries {
					err := ledger.Replace(e.Index, e)
					if err != nil {
						slog.ErrorContext(b.context, "Error writing block ledger", "error", err)
					}
				}

				// TODO: Delete accounts
			})

			// Recreate the read batch
			clear(entries)
			entries = entries[:0]
			batch.Discard()
			batch = b.db.Begin(false)

		case <-b.context.Done():
			return
		}
	}
}

func (b *BlockLedgerIndexer) Stop() {
	b.cancel()
}
