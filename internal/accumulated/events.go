package accumulated

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (d *Daemon) onDidCommitBlock(event events.DidCommitBlock) error {
	// Subscribe synchronously and begin a batch, but do all the actual
	// processing in goroutines. This way we're guaranteed to get a batch at the
	// right time but the actual hard work is done asynchronously.
	batch := d.db.Begin(false)

	if event.Major > 0 {
		go d.collectSnapshot(batch, event.Major, event.Index)
	}

	go d.checkForStalledDn(batch)
	return nil
}

func (d *Daemon) checkForStalledDn(batch *database.Batch) {
	if d.Config.Accumulate.DnStallLimit == 0 || d.localTm == nil {
		return
	}

	var ledger *protocol.AnchorLedger
	err := batch.Account(d.Config.Accumulate.AnchorPool()).Main().GetAs(&ledger)
	if err != nil {
		d.Logger.Error("Failed to load anchor ledger", "error", err)
		return
	}

	// If all produced anchors have been acknowledged, there's nothing to do
	dnLedger := ledger.Partition(protocol.DnUrl())
	if dnLedger.Produced == 0 || dnLedger.Acknowledged >= dnLedger.Produced {
		return
	}

	status, err := d.localTm.Status(context.Background())
	if err != nil {
		d.Logger.Error("Failed to get Tendermint status", "error", err)
		return
	}

	seqNum := dnLedger.Acknowledged + 1
	chain, err := batch.Account(d.Config.Accumulate.AnchorPool()).AnchorSequenceChain().Get()
	if err != nil {
		d.Logger.Error("Failed to load anchor sequence chain", "error", err)
		return
	}
	hash, err := chain.Entry(int64(seqNum) - 1)
	if err != nil {
		// If the entry isn't found, that's almost certainly because the anchor
		// hasn't been sent yet, so don't do anything
		if !errors.Is(err, errors.StatusNotFound) {
			d.Logger.Error("Failed to load anchor sequence chain entry", "error", err, "entry", seqNum-1)
		}
		return
	}
	txn, err := batch.Transaction(hash).Main().Get()
	if err != nil {
		d.Logger.Error("Failed to load anchor", "error", err, "seq-num", seqNum, "hash", logging.AsHex(hash).Slice(0, 4))
		return
	}
	if txn.Transaction == nil {
		d.Logger.Error("Invalid anchor: not a transaction", "seq-num", seqNum, "hash", logging.AsHex(hash).Slice(0, 4))
		return
	}
	anchor, ok := txn.Transaction.Body.(protocol.AnchorBody)
	if !ok {
		d.Logger.Error("Invalid anchor: not an anchor", "seq-num", dnLedger.Acknowledged, "hash", logging.AsHex(hash).Slice(0, 4), "type", txn.Transaction.Body.Type())
		return
	}

	sentIndex := anchor.GetPartitionAnchor().MinorBlockIndex
	currIndex := status.SyncInfo.LatestBlockHeight
	if currIndex-int64(sentIndex) > int64(d.Config.Accumulate.DnStallLimit) {
		d.Logger.Error("Fatal error: DN has stalled")
		err = d.Stop()
		if err != nil {
			d.Logger.Error("Error while stopping", "error", err)
			return
		}
	}
}
