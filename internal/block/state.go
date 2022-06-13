package block

import (
	"time"

	"github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// BlockMeta is metadata about a block.
type BlockMeta struct {
	IsLeader   bool
	Index      uint64
	Time       time.Time
	CommitInfo *types.LastCommitInfo
	Evidence   []types.Evidence
}

// BlockState tracks various metrics of a block of transactions as they are
// executed.
type BlockState struct {
	OpenedMajorBlock   bool
	MakeMajorBlock     uint64
	MakeMajorBlockTime time.Time
	Delivered          uint64
	Signed             uint64
	ValidatorsUpdates  []chain.ValidatorUpdate
	ProducedTxns       []*protocol.Transaction
	ChainUpdates       chain.ChainUpdates
}

// Empty returns true if nothing happened during the block.
func (s *BlockState) Empty() bool {
	return !s.OpenedMajorBlock &&
		s.Delivered == 0 &&
		s.Signed == 0 &&
		len(s.ValidatorsUpdates) == 0 &&
		len(s.ProducedTxns) == 0 &&
		len(s.ChainUpdates.Entries) == 0
}

type ProcessSignatureState struct {
}

func (s *ProcessSignatureState) Merge(r *ProcessSignatureState) {
}

func (s *BlockState) MergeSignature(r *ProcessSignatureState) {
	s.Signed++
}

func (s *BlockState) MergeTransaction(r *chain.ProcessTransactionState) {
	s.Delivered++
	s.ValidatorsUpdates = append(s.ValidatorsUpdates, r.ValidatorsUpdates...)
	s.ProducedTxns = append(s.ProducedTxns, r.ProducedTxns...)
	s.ChainUpdates.Merge(&r.ChainUpdates)
	if r.MakeMajorBlock > 0 {
		s.MakeMajorBlock = r.MakeMajorBlock
		s.MakeMajorBlockTime = r.MakeMajorBlockTime
	}
}
