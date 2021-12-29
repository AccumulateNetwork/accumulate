package chain

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type governor struct {
	ExecutorOptions
	logger     logging.OptionalLogger
	dispatcher *dispatcher
	started    int32
	messages   chan interface{}
	done       chan struct{}
}

type govStop struct{}

type govDidBeginBlock struct {
	height int64
	time   time.Time
}

type govDidCommit struct {
	mirrorAdi   bool
	height      int64
	time        time.Time
	blockMeta   *BlockMetadata
	rootAnchor  []byte
	synthAnchor []byte
}

func newGovernor(opts ExecutorOptions) *governor {
	g := new(governor)
	g.ExecutorOptions = opts
	g.messages = make(chan interface{})
	g.done = make(chan struct{})
	g.dispatcher = newDispatcher(opts)
	g.logger.L = opts.Logger
	g.logger.L = g.logger.With("module", "governor")
	return g
}

func (g *governor) Start() error {
	if !atomic.CompareAndSwapInt32(&g.started, 0, 1) {
		return errors.New("already started")
	}

	go g.run()
	return nil
}

func (g *governor) DidBeginBlock(isLeader bool, height int64, time time.Time) {
	g.logger.Debug("Did begin block", "height", height, "time", time)

	if !isLeader {
		// Nothing to do if we're not the leader
		return
	}

	select {
	case g.messages <- govDidBeginBlock{
		height: height,
		time:   time,
	}:
	case <-g.done:
	}
}

func (g *governor) DidCommit(batch *database.Batch, isLeader, mirrorAdi bool, height int64, time time.Time, blockMeta *BlockMetadata) error {
	g.logger.Debug("Did commit block", "height", height, "time", time)

	if !isLeader {
		// Nothing to do if we're not the leader
		return nil
	}

	ledger := batch.Record(g.Network.NodeUrl().JoinPath(protocol.Ledger))
	rootChain, err := ledger.Chain(protocol.MinorRootChain)
	if err != nil {
		return err
	}

	synthChain, err := ledger.Chain(protocol.SyntheticChain)
	if err != nil {
		return err
	}

	select {
	case g.messages <- govDidCommit{
		mirrorAdi:   mirrorAdi,
		height:      height,
		time:        time,
		blockMeta:   blockMeta,
		rootAnchor:  rootChain.Anchor(),
		synthAnchor: synthChain.Anchor(),
	}:
	case <-g.done:
	}
	return nil
}

func (g *governor) Stop() error {
	select {
	case g.messages <- govStop{}:
		return nil
	case <-g.done:
		return errors.New("already stopped")
	}
}

func (g *governor) run() {
	defer close(g.done)

	// In order for other BVCs to be able to validate the synthetic transaction,
	// a wrapped signed version must be resubmitted to this BVC network and the
	// UNSIGNED version of the transaction along with the Leader address will be
	// stored in a SynthChain in the SMT on this BVC. The BVCs will validate the
	// synth transaction against the receipt and EVERYONE will then send out the
	// wrapped TX along with the proof from the directory chain. If by end block
	// there are still unprocessed synthetic TX's the current leader takes over,
	// invalidates the previous leader's signed tx, signs the unprocessed synth
	// tx, and tries again with the new leader. By EVERYONE submitting the
	// leader signed synth tx to the designated BVC network it takes advantage
	// of the flood-fill gossip network tendermint will provide and ensure the
	// synth transaction will be picked up.

	for msg := range g.messages {
		// Reset dispatcher
		g.dispatcher.Reset(context.Background())

		// The governor must be read-only, so we must not commit the
		// database transaction or the state cache. If the governor makes
		// ANY changes, the system will no longer be deterministic.
		batch := g.DB.Begin()
		defer batch.Discard()

		switch msg := msg.(type) {
		case govStop:
			return

		case govDidBeginBlock:
			// Should we do anything at begin?

		case govDidCommit:
			// Mirror the subnet's ADI
			if msg.mirrorAdi {
				g.sendMirror(batch)
			}

			// Create an anchor for the block
			g.sendAnchor(batch, &msg)

			// Load ledger
			ledger := batch.Record(g.Network.NodeUrl().JoinPath(protocol.Ledger))
			ledgerState := new(protocol.InternalLedger)
			err := ledger.GetStateAs(ledgerState)
			if err != nil {
				// If we can't load the ledger, the node is fubared
				panic(fmt.Errorf("failed to load the ledger: %v", err))
			}

			// Sign and send produced synthetic transactions
			g.signTransactions(batch, ledgerState)
			g.sendTransactions(batch, ledgerState)
		}

		err := g.dispatcher.Send(context.Background())
		if err != nil {
			g.logger.Error("Failed to dispatch transactions", "error", err)
		}
	}
}

func (g *governor) signTransactions(batch *database.Batch, ledger *protocol.InternalLedger) {
	if len(ledger.Synthetic.Unsigned) == 0 {
		return
	}

	body := new(protocol.InternalTransactionsSigned)
	body.Transactions = make([]protocol.TransactionSignature, 0, len(ledger.Synthetic.Unsigned))

	// For each unsigned synthetic transaction
	for _, txid := range ledger.Synthetic.Unsigned {
		// Load it
		tx, err := batch.Transaction(txid[:]).GetState()
		if err != nil {
			g.logger.Error("Failed to load pending transaction", "txid", logging.AsHex(txid), "error", err)
			continue
		}
		if tx.Transaction == nil {
			g.logger.Error("Transaction has no payload", "txid", logging.AsHex(txid))
			continue
		}

		var typ types.TransactionType
		_ = typ.UnmarshalBinary(*tx.Transaction)
		g.logger.Info("Signing synth txn", "txid", logging.AsHex(txid), "type", typ)

		// Sign it
		ed := new(transactions.ED25519Sig)
		ed.PublicKey = g.Key[32:]
		err = ed.Sign(tx.SigInfo.Nonce, g.Key, txid[:])
		if err != nil {
			g.logger.Error("Failed to sign pending transaction", "txid", logging.AsHex(txid), "error", err)
			continue
		}

		// Add it to the list
		var sig protocol.TransactionSignature
		sig.Transaction = txid
		sig.Signature = ed
		body.Transactions = append(body.Transactions, sig)
	}

	g.sendInternal(batch, body)
}

func (g *governor) sendTransactions(batch *database.Batch, ledger *protocol.InternalLedger) {
	if len(ledger.Synthetic.Unsent) == 0 {
		return
	}

	body := new(protocol.InternalTransactionsSent)
	body.Transactions = make([][32]byte, 0, len(ledger.Synthetic.Unsent))

	// For each unsent synthetic transaction
	for _, id := range ledger.Synthetic.Unsent {
		// Load it
		pending, _, signatures, err := batch.Transaction(id[:]).Get()
		if err != nil {
			g.logger.Error("Failed to load pending transaction", "txid", logging.AsHex(id), "error", err)
			continue
		}

		if len(signatures) == 0 {
			g.logger.Error("Transaction has no signatures!", "txid", logging.AsHex(id))
			continue
		}

		// Convert it back to a transaction
		tx := pending.Restore()
		tx.Signature = signatures

		// Marshal it
		raw, err := tx.Marshal()
		if err != nil {
			g.logger.Error("Failed to marshal pending transaction", "txid", logging.AsHex(id), "error", err)
			continue
		}

		// Parse the URL
		u, err := url.Parse(tx.SigInfo.URL)
		if err != nil {
			g.logger.Error("Invalid pending transaction URL", "txid", logging.AsHex(id), "error", err, "url", tx.SigInfo.URL)
			continue
		}

		// Send it
		g.logger.Info("Sending synth txn", "actor", u.String(), "txid", logging.AsHex(tx.TransactionHash()))
		g.dispatcher.BroadcastTxAsync(context.Background(), u, raw)
		body.Transactions = append(body.Transactions, id)
	}

	g.sendInternal(batch, body)
}

func (g *governor) sendAnchor(batch *database.Batch, msg *govDidCommit) {
	// Make a copy
	meta := msg.blockMeta.Deliver

	// Don't count synthetic anchor transactions
	submitted := meta.Submitted
	meta.Submitted = make([]*SubmittedTransaction, 0, len(submitted))
	for _, tx := range submitted {
		if tx.Body.GetType() == types.TxTypeSyntheticAnchor {
			continue
		}
		meta.Submitted = append(meta.Submitted, tx)
	}

	// Don't create an anchor transaction if no records were updated and no
	// synthetic transactions (other than synthetic anchors) were produced
	if meta.Empty() {
		return
	}

	body := new(protocol.SyntheticAnchor)
	body.Source = g.Network.NodeUrl().String()
	body.Index = msg.height
	body.Timestamp = msg.time
	copy(body.Root[:], batch.RootHash())
	body.Chains = make([][32]byte, len(msg.blockMeta.Deliver.Updated))
	copy(body.ChainAnchor[:], msg.rootAnchor)
	copy(body.SynthTxnAnchor[:], msg.synthAnchor)

	g.logger.Info("Creating anchor txn", "root", logging.AsHex(body.Root), "chains", logging.AsHex(body.ChainAnchor), "synth", logging.AsHex(body.SynthTxnAnchor))

	for i, u := range msg.blockMeta.Deliver.Updated {
		body.Chains[i] = u.ResourceChain32()
		g.logger.Info("Anchor includes", "id", logging.AsHex(body.Chains[i]), "url", u.String())
	}

	txns := new(protocol.InternalSendTransactions)
	switch g.Network.Type {
	case config.Directory:
		// Send anchors from DN to all BVNs
		txns.Transactions = make([]protocol.SendTransaction, len(g.Network.BvnNames))
		for i, bvn := range g.Network.BvnNames {
			txns.Transactions[i] = protocol.SendTransaction{
				Recipient: protocol.BvnUrl(bvn),
				Payload:   body,
			}
		}

	case config.BlockValidator:
		// Send anchor from BVN to DN
		txns.Transactions = []protocol.SendTransaction{{
			Recipient: protocol.DnUrl(),
			Payload:   body,
		}}
	}

	g.sendInternal(batch, txns)
}

func (g *governor) sendMirror(batch *database.Batch) {
	mirror := new(protocol.SyntheticMirror)

	nodeUrl := g.Network.NodeUrl()
	rec, err := mirrorRecord(batch, nodeUrl)
	if err != nil {
		g.logger.Error("Failed to mirror ADI", "error", err, "url", nodeUrl)
		return
	}
	mirror.Objects = append(mirror.Objects, rec)

	md, err := loadDirectoryMetadata(batch, nodeUrl.ResourceChain())
	if err != nil {
		g.logger.Error("Failed to load directory", "error", err, "url", nodeUrl)
		return
	}

	for i := uint64(0); i < md.Count; i++ {
		s, err := loadDirectoryEntry(batch, nodeUrl.ResourceChain(), i)
		if err != nil {
			g.logger.Error("Failed to load directory entry", "error", err, "url", nodeUrl, "index", i)
			return
		}

		u, err := url.Parse(s)
		if err != nil {
			g.logger.Error("Invalid directory entry", "error", err, "url", nodeUrl, "index", i)
			return
		}

		rec, err := mirrorRecord(batch, u)
		if err != nil {
			g.logger.Error("Failed to mirror directory entry", "error", err, "url", nodeUrl, "index", i)
			return
		}
		mirror.Objects = append(mirror.Objects, rec)
	}

	txns := new(protocol.InternalSendTransactions)
	switch g.Network.Type {
	case config.Directory:
		txns.Transactions = make([]protocol.SendTransaction, len(g.Network.BvnNames))
		for i, bvn := range g.Network.BvnNames {
			txns.Transactions[i] = protocol.SendTransaction{
				Recipient: protocol.BvnUrl(bvn),
				Payload:   mirror,
			}
		}

	case config.BlockValidator:
		txns.Transactions = []protocol.SendTransaction{{
			Recipient: protocol.DnUrl(),
			Payload:   mirror,
		}}
	}

	g.sendInternal(batch, txns)
}

func mirrorRecord(batch *database.Batch, u *url.URL) (protocol.AnchoredRecord, error) {
	var arec protocol.AnchoredRecord

	rec := batch.Record(u)
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

func loadDirectoryMetadata(batch *database.Batch, chainId []byte) (*protocol.DirectoryIndexMetadata, error) {
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

func loadDirectoryEntry(batch *database.Batch, chainId []byte, index uint64) (string, error) {
	b, err := batch.RecordByID(chainId).Index("Directory", index).Get()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (g *governor) sendInternal(batch *database.Batch, body protocol.TransactionPayload) {
	// Construct the signature transaction
	tx, err := g.buildSynthTxn(g.Network.NodeUrl().JoinPath(protocol.Ledger), body, batch)
	if err != nil {
		g.logger.Error("Failed to build internal transaction", "error", err)
		return
	}

	// Sign it
	ed := new(transactions.ED25519Sig)
	tx.Signature = append(tx.Signature, ed)
	ed.PublicKey = g.Key[32:]
	err = ed.Sign(tx.SigInfo.Nonce, g.Key, tx.TransactionHash())
	if err != nil {
		g.logger.Error("Failed to sign internal transaction", "error", err)
		return
	}

	// Marshal it
	data, err := tx.Marshal()
	if err != nil {
		g.logger.Error("Failed to marshal internal transaction", "error", err)
		return
	}

	// Send it
	g.logger.Info("Sending internal txn", "txid", logging.AsHex(tx.TransactionHash()), "type", body.GetType())
	g.dispatcher.BroadcastTxAsyncLocal(context.TODO(), data)
}
