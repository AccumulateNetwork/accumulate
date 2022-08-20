package block

import (
	"time"

	"github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/execute"
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
	ProducedTxns       []*protocol.Transaction
	ChainUpdates       execute.ChainUpdates
	ReceivedAnchors    []*execute.ReceivedAnchor

	Anchor *BlockAnchorState
}

// BlockAnchorState is used to construc the anchor for the block.
type BlockAnchorState struct {
	ShouldOpenMajorBlock bool
	OpenMajorBlockTime   time.Time
}

// Empty returns true if nothing happened during the block.
func (s *BlockState) Empty() bool {
	return !s.OpenedMajorBlock &&
		s.Anchor == nil &&
		s.Delivered == 0 &&
		s.Signed == 0 &&
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

func (s *BlockState) MergeTransaction(r *execute.ProcessTransactionState) {
	s.Delivered++
	s.ProducedTxns = append(s.ProducedTxns, r.ProducedTxns...)
	s.ChainUpdates.Merge(&r.ChainUpdates)
	s.ReceivedAnchors = append(s.ReceivedAnchors, r.ReceivedAnchors...)
	if r.MakeMajorBlock > 0 {
		s.MakeMajorBlock = r.MakeMajorBlock
		s.MakeMajorBlockTime = r.MakeMajorBlockTime
	}
}
