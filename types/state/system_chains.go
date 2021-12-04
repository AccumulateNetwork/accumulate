package state

import (
	"bytes"
	"sort"
	"time"
)

func (s *StateDB) MinorAnchorChain() (*AnchorChainManager, error) {
	mgr, err := s.ManageChain("Anchor", "Minor")
	if err != nil {
		return nil, err
	}
	return &AnchorChainManager{*mgr}, nil
}

func (s *StateDB) SynthTxidChain() (*ChainManager, error) {
	return s.ManageChain("Synthetic", "Txid")
}

func (tx *DBTransaction) writeSynthChain(blockIndex int64) error {
	// Collect and sort a list of all synthetic transaction IDs
	var txids [][32]byte
	seen := map[[32]byte]bool{}
	for _, txns := range tx.transactions.synthTxMap {
		for _, txn := range txns {
			txid := txn.TxId.AsBytes32()
			if seen[txid] {
				continue
			}
			seen[txid] = true
			txids = append(txids, txid)
		}
	}
	sort.Slice(txids, func(i, j int) bool {
		return bytes.Compare(txids[i][:], txids[j][:]) < 0
	})

	if len(txids) == 0 {
		return nil
	}

	// Load the chain
	mgr, err := tx.state.SynthTxidChain()
	if err != nil {
		return err
	}

	// Add all of the synth txids
	for _, txid := range txids {
		err = mgr.AddEntry(txid[:])
		if err != nil {
			return err
		}
	}

	// Update the record
	err = mgr.UpdateAs(&SyntheticTransactionChain{
		Index: blockIndex,
		Count: int64(len(txids)),
	})
	if err != nil {
		return err
	}

	return nil
}

func (tx *DBTransaction) writeAnchorChain(blockIndex int64, timestamp time.Time) error {
	// Collect a list of all chain IDs
	chains := make([][32]byte, 0, len(tx.updates))
	for id := range tx.updates {
		chains = append(chains, id)
	}

	// Load the chain
	mgr, err := tx.state.MinorAnchorChain()
	if err != nil {
		return err
	}

	// Update the chain. Even if no records changed, we need to update the
	// anchor record. Otherwise, the block height does not get updated and
	// everything breaks.
	return mgr.Update(blockIndex, timestamp, chains)
}
