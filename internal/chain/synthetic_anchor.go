package chain

import (
	"bytes"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/managed"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type SyntheticAnchor struct {
	Network *config.Network
}

func (SyntheticAnchor) Type() types.TxType { return types.TxTypeSyntheticAnchor }

func (x SyntheticAnchor) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	// Unpack the payload
	body := new(protocol.SyntheticAnchor)
	err := tx.As(body)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %v", err)
	}

	// Verify the origin
	if _, ok := st.Origin.(*protocol.Anchor); !ok {
		return nil, fmt.Errorf("invalid origin record: want %v, got %v", types.AccountTypeAnchor, st.Origin.Header().Type)
	}

	// Check the source URL
	source, err := url.Parse(body.Source)
	if err != nil {
		return nil, fmt.Errorf("invalid source: %v", err)
	}
	name, ok := protocol.ParseBvnUrl(source)
	var fromDirectory bool
	switch {
	case ok:
		name = "bvn-" + name
	case protocol.IsDnUrl(source):
		name, fromDirectory = "dn", true
	default:
		return nil, fmt.Errorf("invalid source: not a BVN or the DN")
	}

	if body.Receipt.Start != nil {
		// If we got a receipt, verify it
		err = x.verifyReceipt(st, body)
		if err != nil {
			return nil, err
		}

		if fromDirectory {
			st.AddDirectoryAnchor(body)
		}
	}

	// Add the anchor to the chain
	err = st.AddChainEntry(st.OriginUrl, name, protocol.ChainTypeAnchor, body.RootAnchor[:], body.RootIndex, body.Block)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (SyntheticAnchor) verifyReceipt(st *StateManager, body *protocol.SyntheticAnchor) error {
	// Get the merkle state at the specified index
	chainName := protocol.MinorRootChain
	if body.Major {
		chainName = protocol.MajorRootChain
	}
	rootChain, err := st.ReadChain(st.nodeUrl.JoinPath(protocol.Ledger), chainName)
	if err != nil {
		return fmt.Errorf("failed to open ledger %s chain: %v", chainName, err)
	}
	ms, err := rootChain.State(int64(body.SourceIndex))
	if err != nil {
		return fmt.Errorf("failed to get state %d of ledger %s chain: %v", body.SourceIndex, chainName, err)
	}

	// Verify the start matches the root chain anchor
	if !bytes.Equal(ms.GetMDRoot(), body.Receipt.Start) {
		return fmt.Errorf("receipt start does match root anchor at %d", body.RootIndex)
	}

	// Calculate receipt end
	hash := managed.Hash(body.Receipt.Start)
	for _, entry := range body.Receipt.Entries {
		if entry.Right {
			hash = hash.Combine(managed.Sha256, entry.Hash)
		} else {
			hash = managed.Hash(entry.Hash).Combine(managed.Sha256, hash)
		}
	}

	// Verify the end matches what we received
	if !bytes.Equal(hash, body.RootAnchor[:]) {
		return fmt.Errorf("receipt end does match received root")
	}

	return nil
}
