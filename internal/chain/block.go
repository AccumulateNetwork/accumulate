package chain

import (
	"fmt"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/abci"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// BlockMeta is metadata about a block.
type BlockMeta struct {
	IsLeader bool
	Index    int64
	Time     time.Time
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

// BlockState tracks various metrics of a block of transactions as they are
// executed.
type BlockState struct {
	chains map[string]*ChainUpdate

	Delivered            uint64
	SynthSigned          uint64
	SynthSent            uint64
	ValidatorsUpdates    []abci.ValidatorUpdate
	ProducedTxns         []*protocol.Transaction
	ChainUpdates         []ChainUpdate
	SynthReceiptEnvelope *SynthReceiptEnvelope
}

type SynthReceiptEnvelope struct {
	DestUrl          *url.URL
	SyntheticReceipt *protocol.SyntheticReceipt
}

// Empty returns true if nothing happened during the block.
func (s *BlockState) Empty() bool {
	return s.Delivered == 0 &&
		s.SynthSigned == 0 &&
		s.SynthSent == 0 &&
		len(s.ValidatorsUpdates) == 0 &&
		len(s.ProducedTxns) == 0 &&
		len(s.ChainUpdates) == 0
}

// Merge merges pending block changes into the block state.
func (s *BlockState) Merge(r *BlockState) {
	s.Delivered += r.Delivered
	s.SynthSigned += r.SynthSigned
	s.SynthSent += r.SynthSent
	s.ValidatorsUpdates = append(s.ValidatorsUpdates, r.ValidatorsUpdates...)
	s.ProducedTxns = append(s.ProducedTxns, r.ProducedTxns...)

	for _, u := range r.ChainUpdates {
		s.DidUpdateChain(u)
	}
}

// DidUpdateChain records a chain update.
func (s *BlockState) DidUpdateChain(update ChainUpdate) {
	if s.chains == nil {
		s.chains = map[string]*ChainUpdate{}
	}

	str := strings.ToLower(fmt.Sprintf("%s#chain/%s", update.Account, update.Name))
	ptr, ok := s.chains[str]
	if ok {
		*ptr = update
		return
	}

	i := len(s.ChainUpdates)
	s.ChainUpdates = append(s.ChainUpdates, update)
	s.chains[str] = &s.ChainUpdates[i]
}

// DidProduceTxn records a produced transaction.
func (s *BlockState) DidProduceTxn(url *url.URL, body protocol.TransactionBody) {
	txn := new(protocol.Transaction)
	txn.Header.Principal = url
	txn.Body = body
	s.ProducedTxns = append(s.ProducedTxns, txn)
}
