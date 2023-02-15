package vdk

import (
	abcitypes "github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Executor struct {
	execute.Executor
}

func (e *Executor) LoadStateRoot(*database.Batch) ([]byte, error) {
	return nil, nil
}

// RestoreSnapshot restores the database from a snapshot.
func (e *Executor) RestoreSnapshot(database.Beginner, ioutil2.SectionReader) error {
	return nil
}

// InitChainValidators validates the given initial validators and returns
// any additional validators.
func (e *Executor) InitChainValidators(initVal []abcitypes.ValidatorUpdate) (additional [][]byte, err error) {
	return nil, nil
}

// Validate validates a set of messages.
func (e *Executor) Validate(*database.Batch, []messaging.Message) ([]*protocol.TransactionStatus, error) {
	return nil, nil
}

// Begin begins a Tendermint block.
func (e *Executor) Begin(execute.BlockParams) (execute.Block, error) {
	return nil, nil
}
