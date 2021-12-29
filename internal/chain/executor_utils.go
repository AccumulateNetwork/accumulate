package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

func (m *Executor) loadDirectoryMetadata(batch *database.Batch, chainId []byte) (*protocol.DirectoryIndexMetadata, error) {
	b, err := batch.RecordByID(chainId).Index("Directory", "Metadata").Get()
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

func (m *Executor) loadDirectoryEntry(batch *database.Batch, chainId []byte, index uint64) (string, error) {
	b, err := batch.RecordByID(chainId).Index("Directory", index).Get()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (m *Executor) mirrorADIs(urls ...*url.URL) (*protocol.SyntheticMirror, error) {
	mirror := new(protocol.SyntheticMirror)

	for _, u := range urls {
		rec, err := m.mirrorRecord(u)
		if err != nil {
			return nil, err
		}

		mirror.Objects = append(mirror.Objects, rec)

		chainId := u.ResourceChain()
		md, err := m.loadDirectoryMetadata(m.blockBatch, chainId)
		if err != nil {
			return nil, fmt.Errorf("failed to load directory for %q: %v", u, err)
		}

		for i := uint64(0); i < md.Count; i++ {
			s, err := m.loadDirectoryEntry(m.blockBatch, chainId, i)
			if err != nil {
				return nil, fmt.Errorf("failed to load directory entry %d", i)
			}

			u, err := url.Parse(s)
			if err != nil {
				return nil, fmt.Errorf("invalid url %q: %v", s, err)
			}

			rec, err := m.mirrorRecord(u)
			if err != nil {
				return nil, err
			}

			mirror.Objects = append(mirror.Objects, rec)
		}
	}

	return mirror, nil
}

func (m *Executor) mirrorRecord(u *url.URL) (protocol.AnchoredRecord, error) {
	var arec protocol.AnchoredRecord

	rec := m.blockBatch.Record(u)
	state, err := rec.GetState()
	if err != nil {
		return arec, fmt.Errorf("failed to load %q: %v", u, err)
	}

	chain, err := rec.Chain(protocol.MainChain)
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

func (m *Executor) synthTxnsLastBlock(blockIndex int64) ([][]byte, error) {
	// Load the chain state
	synth := m.blockBatch.Record(m.Network.NodeUrl().JoinPath(protocol.Synthetic))
	head := state.NewSyntheticTransactionChain()
	err := synth.GetStateAs(head)
	if err != nil {
		return nil, err
	}

	// Did anything happen last block?
	if head.Index != blockIndex {
		return nil, nil
	}

	// Load the synth txn chain
	chain, err := synth.Chain(protocol.MainChain)
	if err != nil {
		return nil, err
	}

	// Retrieve transactions from the previous block
	height := chain.Height()
	return chain.Entries(height-head.Count, height)
}
