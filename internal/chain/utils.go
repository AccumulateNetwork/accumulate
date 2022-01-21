package chain

import (
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/indexing"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/managed"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types"
)

func  addChainEntry(nodeUrl *url.URL, batch *database.Batch, account *url.URL, name string, typ protocol.ChainType, entry []byte, sourceIndex uint64) error {
	// Check if the account exists
	_, err := batch.Account(account).GetState()
	if err != nil {
		return err
	}

	// Add an entry to the chain
	chain, err := batch.Account(account).Chain(name, typ)
	if err != nil {
		return err
	}

	index := chain.Height()
	err = chain.AddEntry(entry)
	if err != nil {
		return err
	}

	// Update the ledger
	return didAddChainEntry(nodeUrl, batch, account, name, typ, entry, uint64(index), sourceIndex)
}

func didAddChainEntry(nodeUrl *url.URL, batch *database.Batch, u *url.URL, name string, typ protocol.ChainType, entry []byte, index, sourceIndex uint64) error {
	if name == protocol.SyntheticChain && typ == protocol.ChainTypeTransaction {
		err := indexing.BlockState(batch, u).DidProduceSynthTxn(&indexing.BlockStateSynthTxnEntry{
			Transaction: entry,
			ChainEntry: index,
		})
		if err != nil {
			return err
		}
	}

	ledger := batch.Account(nodeUrl.JoinPath(protocol.Ledger))
	ledgerState := protocol.NewInternalLedger()
	err := ledger.GetStateAs(ledgerState)
	switch {
	case err == nil:
		// OK
	case errors.Is(err, storage.ErrNotFound):
		// For genesis
		return nil
	default:
		return err
	}

	s := u.String()
	if u.Path == "/foo/tokens" {
		println(s)
	}

	var meta protocol.AnchorMetadata
	meta.Name = name
	meta.Type = typ
	meta.Account = u
	meta.Index = index
	meta.SourceIndex = sourceIndex
	meta.Entry = entry
	ledgerState.Updates = append(ledgerState.Updates, meta)
	return ledger.PutState(ledgerState)
}

func loadDirectoryMetadata(batch *database.Batch, chainId []byte) (*protocol.DirectoryIndexMetadata, error) {
	b, err := batch.AccountByID(chainId).Index("Directory", "Metadata").Get()
	if err != nil {
		return nil, err
	}

	md := new(protocol.DirectoryIndexMetadata)
	err = md.UnmarshalBinary(b)
	if err != nil {
		return nil, err
	}

	return md, nil
}

func loadDirectoryEntry(batch *database.Batch, chainId []byte, index uint64) (string, error) {
	b, err := batch.AccountByID(chainId).Index("Directory", index).Get()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func mirrorRecord(batch *database.Batch, u *url.URL) (protocol.AnchoredRecord, error) {
	var arec protocol.AnchoredRecord

	rec := batch.Account(u)
	state, err := rec.GetState()
	if err != nil {
		return arec, fmt.Errorf("failed to load %q: %v", u, err)
	}

	chain, err := rec.ReadChain(protocol.MainChain)
	if err != nil {
		return arec, fmt.Errorf("failed to load main chain of %q: %v", u, err)
	}

	arec.Record, err = state.MarshalBinary()
	if err != nil {
		return arec, fmt.Errorf("failed to marshal %q: %v", u, err)
	}

	copy(arec.Anchor[:], chain.Anchor())
	return arec, nil
}

func buildProof(batch *database.Batch, u *protocol.AnchorMetadata, rootChain *database.Chain, rootIndex, rootHeight int64) (*managed.Receipt, error) {
	anchorChain, err := batch.Account(u.Account).ReadChain(u.Name)
	if err != nil {
		return nil, err
	}

	anchorHeight := anchorChain.Height()
	r1, err := anchorChain.Receipt(int64(u.Index), anchorHeight-1)
	if err != nil {
		return nil, err
	}

	r2, err := rootChain.Receipt(rootIndex, rootHeight-1)
	if err != nil {
		return nil, err
	}

	r, err := r1.Combine(r2)
	if err != nil {
		return nil, err
	}

	return r, nil
}

func synthCountExceptAnchors(batch *database.Batch, ledger *protocol.InternalLedger) int {
	var count int
	for _, hash := range ledger.Synthetic.Produced {
		txn, err := batch.Transaction(hash[:]).GetState()
		if err != nil {
			count++
			continue
		}

		if txn.TxType() != types.TxTypeSyntheticAnchor {
			count++
			continue
		}
	}
	return count
}
