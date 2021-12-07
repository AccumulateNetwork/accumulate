package state

import (
	"time"
)

func (s *StateDB) MinorAnchorChain() (*AnchorChainManager, error) {
	mgr, err := s.ManageChain("Anchor", "Minor")
	if err != nil {
		return nil, err
	}
	return &AnchorChainManager{*mgr}, nil
}

func (s *StateDB) SynthTxidChain() (*SynthChainManager, error) {
	mgr, err := s.ManageChain("Synthetic", "Txid")
	if err != nil {
		return nil, err
	}
	return &SynthChainManager{*mgr}, nil
}

func (tx *DBTransaction) writeSynthChain(blockIndex int64) error {
	// Collect synthetic transaction IDs
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

	// Load the chain
	mgr, err := tx.state.SynthTxidChain()
	if err != nil {
		return err
	}

	// Update the chain
	return mgr.Update(blockIndex, txids)
}

func (tx *DBTransaction) writeAnchorChain(blockIndex int64, timestamp time.Time) error {
	// Collect chain IDs
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
