package node

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Executor struct {
	x execute.Executor
}

type BlockParams struct {
	b execute.BlockParams
}

type ValidatorUpdate struct {
	v execute.ValidatorUpdate
}

type DataSet struct {
	d logging.DataSet
}

type Block struct {
	b *execute.Block
}

func (x *Executor) EnableTimers() {
	x.x.EnableTimers()
}

func (x *Executor) StoreBlockTimers(ds *DataSet) {
	x.x.StoreBlockTimers(&ds.d)
}

// LastBlock returns the height and hash of the last block.
func (x *Executor) LastBlock() (bp *BlockParams, h [32]byte, err error) {
	bp = new(BlockParams)
	b, h, err := x.x.LastBlock()
	bp.b = *b
	return bp, h, err
}

// Restore restores the database from a snapshot, validates the initial
// validators, and returns any additional validators.
func (x *Executor) Restore(snapshot ioutil2.SectionReader, validators []*ValidatorUpdate) (additional []*ValidatorUpdate, err error) {
	var vu []*execute.ValidatorUpdate
	for _, v := range validators {
		vu = append(vu, &v.v)
	}
	r, err := x.x.Restore(snapshot, vu)
	if err == nil {
		for _, v := range r {
			additional = append(additional, &ValidatorUpdate{*v})
		}
	}
	return additional, err
}

// Validate validates a set of messages.
func (x *Executor) Validate(messages []messaging.Message, recheck bool) ([]*protocol.TransactionStatus, error) {
	return x.x.Validate(messages, recheck)
}

// Begin begins a Tendermint block.
func (x *Executor) Begin(bp BlockParams) (Block, error) {
	b, err := x.x.Begin(bp.b)
	return Block{b: &b}, err
}

type Event struct {
	events.Event
}

type DidCommitBlock struct {
	DidCommit events.DidCommitBlock
}

type DidSaveSnapshot struct {
	DidSave events.DidSaveSnapshot
}

type FatalError struct {
	Fatal events.FatalError
}

func (e FatalError) Error() string { return e.Fatal.Err.Error() }
func (e FatalError) Unwrap() error { return e.Fatal.Err }
