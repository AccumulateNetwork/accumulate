package chain

import (
	"fmt"
	"strings"
	"time"

	"github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// BeginBlockRequest is the input parameter to Chain.BeginBlock.
type BeginBlockRequest struct {
	IsLeader   bool
	Height     int64
	Time       time.Time
	CommitInfo *types.LastCommitInfo
	Evidence   []types.Evidence
}

// BeginBlockResponse is the return value of Chain.BeginBlock.
type BeginBlockResponse struct{}

// SynthTxnReference is a reference to a produced synthetic transaction.
type SynthTxnReference struct {
	Type  uint64   `json:"type,omitempty" form:"type" query:"type" validate:"required"`
	Hash  [32]byte `json:"hash,omitempty" form:"hash" query:"hash" validate:"required"`
	Url   string   `json:"url,omitempty" form:"url" query:"url" validate:"required,acc-url"`
	TxRef [32]byte `json:"txRef,omitempty" form:"txRef" query:"txRef" validate:"required"`
}

type ValidatorUpdate struct {
	PubKey  ed25519.PubKey
	Enabled bool
}

// EndBlockRequest is the input parameter to Chain.EndBlock
type EndBlockRequest struct{}

type EndBlockResponse struct {
	ValidatorsUpdates []ValidatorUpdate
}

type Block struct {
	BlockMeta
	State  BlockState
	Anchor *protocol.SyntheticAnchor
	Batch  *database.Batch
}

// BlockMeta is metadata about a block.
type BlockMeta struct {
	IsLeader   bool
	Index      int64
	Time       time.Time
	CommitInfo *types.LastCommitInfo
	Evidence   []types.Evidence
}

// BlockState tracks various metrics of a block of transactions as they are
// executed.
type BlockState struct {
	Delivered         uint64
	Signed            uint64
	SynthSigned       uint64
	SynthSent         uint64
	ValidatorsUpdates []ValidatorUpdate
	ProducedTxns      []*protocol.Transaction
	ChainUpdates      ChainUpdates
}

// Empty returns true if nothing happened during the block.
func (s *BlockState) Empty() bool {
	return s.Delivered == 0 &&
		s.Signed == 0 &&
		s.SynthSigned == 0 &&
		s.SynthSent == 0 &&
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

type ProcessTransactionState struct {
	ValidatorsUpdates []ValidatorUpdate
	ProducedTxns      []*protocol.Transaction
	ChainUpdates      ChainUpdates
}

// DidProduceTxn records a produced transaction.
func (s *ProcessTransactionState) DidProduceTxn(url *url.URL, body protocol.TransactionBody) {
	txn := new(protocol.Transaction)
	txn.Header.Principal = url
	txn.Body = body
	s.ProducedTxns = append(s.ProducedTxns, txn)
}

func (s *ProcessTransactionState) Merge(r *ProcessTransactionState) {
	s.ValidatorsUpdates = append(s.ValidatorsUpdates, r.ValidatorsUpdates...)
	s.ProducedTxns = append(s.ProducedTxns, r.ProducedTxns...)
	s.ChainUpdates.Merge(&r.ChainUpdates)
}

func (s *BlockState) MergeTransaction(r *ProcessTransactionState) {
	s.Delivered++
	s.ValidatorsUpdates = append(s.ValidatorsUpdates, r.ValidatorsUpdates...)
	s.ProducedTxns = append(s.ProducedTxns, r.ProducedTxns...)
	s.ChainUpdates.Merge(&r.ChainUpdates)
}

type ChainUpdates struct {
	chains  map[string]*ChainUpdate
	Entries []ChainUpdate
}

// ChainUpdate records an update to a chain of an account.
type ChainUpdate struct {
	Account     *url.URL
	Name        string
	Type        protocol.ChainType
	Index       uint64
	SourceIndex uint64
	SourceBlock uint64
	Entry       []byte
}

func (c *ChainUpdates) Merge(d *ChainUpdates) {
	for _, u := range d.Entries {
		c.DidUpdateChain(u)
	}
}

// DidUpdateChain records a chain update.
func (c *ChainUpdates) DidUpdateChain(update ChainUpdate) {
	if c.chains == nil {
		c.chains = map[string]*ChainUpdate{}
	}

	str := strings.ToLower(fmt.Sprintf("%s#chain/%s", update.Account, update.Name))
	ptr, ok := c.chains[str]
	if ok {
		*ptr = update
		return
	}

	i := len(c.Entries)
	c.Entries = append(c.Entries, update)
	c.chains[str] = &c.Entries[i]
}
