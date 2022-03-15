package chain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
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
	mirrorAdi  bool
	height     int64
	time       time.Time
	ledger     *protocol.InternalLedger
	rootAnchor []byte
	rootHeight int64
	receipts   map[string]*receiptAndIndex
}

type receiptAndIndex struct {
	Receipt     protocol.Receipt
	Index       int64
	SourceIndex int64
	Block       uint64
	SourceBlock uint64
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
	g.logger.Debug("Block event", "type", "didBegin", "height", height, "time", time)

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

func (g *governor) DidCommit(batch *database.Batch, isLeader, mirrorAdi bool, height int64, time time.Time) error {
	g.logger.Debug("Block event", "type", "didCommit", "height", height, "time", time)

	if !isLeader {
		// Nothing to do if we're not the leader
		return nil
	}

	msg := govDidCommit{
		mirrorAdi: mirrorAdi,
		height:    height,
		time:      time,
	}

	ledger := batch.Account(g.Network.NodeUrl(protocol.Ledger))
	msg.ledger = protocol.NewInternalLedger()
	err := ledger.GetStateAs(msg.ledger)
	if err != nil {
		return err
	}

	rootChain, err := ledger.ReadChain(protocol.MinorRootChain)
	if err != nil {
		return err
	}
	msg.rootAnchor = rootChain.Anchor()
	msg.rootHeight = rootChain.Height()

	// Find BVN anchor chains
	err = g.buildProofs(&msg, batch, rootChain)
	if err != nil {
		return err
	}

	select {
	case g.messages <- msg:
	case <-g.done:
	}
	return nil
}

func (g *governor) buildProofs(msg *govDidCommit, batch *database.Batch, rootChain *database.Chain) error {
	if g.Network.Type != config.Directory {
		return nil
	}

	anchorUrl := g.Network.NodeUrl(protocol.AnchorPool)
	baseIndex := msg.rootHeight - int64(len(msg.ledger.Updates))
	msg.receipts = map[string]*receiptAndIndex{}

	for i, u := range msg.ledger.Updates {
		if u.Type != protocol.ChainTypeAnchor || !u.Account.Equal(anchorUrl) || !strings.HasPrefix(u.Name, "bvn-") {
			continue
		}

		bvn := u.Name[4:]
		if r := msg.receipts[bvn]; r != nil && r.Index > int64(u.Index) {
			continue
		}

		rootIndex := baseIndex + int64(i)
		r, err := buildProof(batch, &u, rootChain, rootIndex, msg.rootHeight)
		if err != nil {
			return err
		}

		var s protocol.Receipt
		s.Start = r.Element
		s.Entries = make([]protocol.ReceiptEntry, len(r.Nodes))
		for i, n := range r.Nodes {
			s.Entries[i] = protocol.ReceiptEntry{Right: n.Right, Hash: n.Hash}
		}

		msg.receipts[bvn] = &receiptAndIndex{s, int64(u.Index), int64(u.SourceIndex), uint64(msg.height), u.SourceBlock}
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
		switch msg := msg.(type) {
		case govStop:
			return

		case govDidBeginBlock:
			// Should we do anything at begin?

		case govDidCommit:
			g.runDidCommit(&msg)
		}
	}
}

func (g *governor) runDidCommit(msg *govDidCommit) {
	// The governor must be read-only, so we must not commit the
	// database transaction or the state cache. If the governor makes
	// ANY changes, the system will no longer be deterministic.
	batch := g.DB.Begin(false)
	defer batch.Discard()

	// TODO This will hit the database with a lot of queries, maybe we shouldn't do this
	produced := countExceptAnchors(batch, msg.ledger.Synthetic.Produced)
	unsigned := countExceptAnchors(batch, msg.ledger.Synthetic.Unsigned)
	unsent := countExceptAnchors(batch, msg.ledger.Synthetic.Unsent)

	g.logger.Info("Did commit",
		"height", msg.height,
		"time", msg.time,
		"mirror", msg.mirrorAdi,
		"updated", len(msg.ledger.Updates),
		"produced", produced,
		"unsigned", unsigned,
		"unsent", unsent,
	)

	// Mirror the subnet's ADI
	if msg.mirrorAdi {
		g.sendMirror(batch)
	}

	// Create an anchor for the block
	g.sendAnchor(batch, msg, produced)

	// Sign and send produced synthetic transactions
	g.signTransactions(batch, msg.ledger)
	g.sendTransactions(batch, msg.ledger)

	// Dispatch transactions asynchronously
	errs := g.dispatcher.Send(context.Background())
	go func() {
		for err := range errs {
			g.logger.Error("Failed to dispatch transactions", "error", err)
		}
	}()
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

		typ := tx.Transaction.GetType()
		if typ != types.TxTypeSyntheticAnchor {
			g.logger.Debug("Signing synth txn", "txid", logging.AsHex(txid), "type", typ)
		}

		// Sign it
		ed := new(protocol.LegacyED25519Signature)
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
		env := pending.Restore()
		env.Signatures = signatures

		// Marshal it
		raw, err := env.MarshalBinary()
		if err != nil {
			g.logger.Error("Failed to marshal pending transaction", "txid", logging.AsHex(id), "error", err)
			continue
		}

		// Send it
		typ := env.Transaction.Type()
		if typ != types.TxTypeSyntheticAnchor {
			g.logger.Debug("Sending synth txn", "origin", env.Transaction.Origin, "txid", logging.AsHex(env.GetTxHash()), "type", typ)
		}
		err = g.dispatcher.BroadcastTxAsync(context.Background(), env.Transaction.Origin, raw)
		if err != nil {
			g.logger.Error("Failed to dispatch transaction", "txid", logging.AsHex(id), "error", err)
			continue
		}
		body.Transactions = append(body.Transactions, id)
	}

	g.sendInternal(batch, body)
}

func (g *governor) sendAnchor(batch *database.Batch, msg *govDidCommit, synthCountExceptAnchors int) {
	// Don't create an anchor transaction if no records were updated and no
	// synthetic transactions (other than synthetic anchors) were produced
	if len(msg.ledger.Updates) == 0 && synthCountExceptAnchors == 0 {
		return
	}

	body := new(protocol.SyntheticAnchor)
	body.Source = g.Network.NodeUrl()
	body.RootIndex = uint64(msg.rootHeight - 1)
	body.Block = uint64(msg.ledger.Index)
	copy(body.RootAnchor[:], msg.rootAnchor)

	kv := []interface{}{"root", logging.AsHex(body.RootAnchor)}
	for i, c := range msg.ledger.Updates {
		kv = append(kv, fmt.Sprintf("[%d]", i))
		switch c.Name {
		case "bpt":
			kv = append(kv, "BPT")
		case "synthetic":
			kv = append(kv, "synthetic")
		default:
			kv = append(kv, fmt.Sprintf("%s#chain/%s", c.Account, c.Name))
		}
	}
	g.logger.Debug("Creating anchor txn", kv...)

	txns := new(protocol.InternalSendTransactions)
	switch g.Network.Type {
	case config.Directory:
		// If we are the dn, we need to include the ACME oracle price
		body.AcmeOraclePrice = msg.ledger.PendingOracle

		// Send anchors from DN to all BVNs
		bvnNames := g.Network.GetBvnNames()
		txns.Transactions = make([]protocol.SendTransaction, len(bvnNames))
		for i, bvn := range bvnNames {
			body := *body
			if r := msg.receipts[bvn]; r != nil {
				body.Receipt = r.Receipt
				body.SourceIndex = uint64(r.SourceIndex)
				body.SourceBlock = r.SourceBlock
			}

			txns.Transactions[i] = protocol.SendTransaction{
				Recipient: protocol.BvnUrl(bvn).JoinPath(protocol.AnchorPool),
				Payload:   &body,
			}
		}

	case config.BlockValidator:
		// Send anchor from BVN to DN
		txns.Transactions = []protocol.SendTransaction{{
			Recipient: protocol.DnUrl().JoinPath(protocol.AnchorPool),
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

	md, err := loadDirectoryMetadata(batch, nodeUrl.AccountID())
	if err != nil {
		g.logger.Error("Failed to load directory", "error", err, "url", nodeUrl)
		return
	}

	for i := uint64(0); i < md.Count; i++ {
		s, err := loadDirectoryEntry(batch, nodeUrl.AccountID(), i)
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
		bvnNames := g.Network.GetBvnNames()
		txns.Transactions = make([]protocol.SendTransaction, len(bvnNames))
		for i, bvn := range bvnNames {
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

func (g *governor) sendInternal(batch *database.Batch, body protocol.TransactionPayload) {
	// Construct the signature transaction
	st := newStateCache(g.Network.NodeUrl(), 0, [32]byte{}, batch)
	env, err := g.buildSynthTxn(st, g.Network.NodeUrl(protocol.Ledger), body)
	if err != nil {
		g.logger.Error("Failed to build internal transaction", "error", err)
		return
	}

	// Sign it
	ed := new(protocol.LegacyED25519Signature)
	env.Signatures = append(env.Signatures, ed)
	ed.PublicKey = g.Key[32:]
	err = ed.Sign(env.Transaction.Nonce, g.Key, env.GetTxHash())
	if err != nil {
		g.logger.Error("Failed to sign internal transaction", "error", err)
		return
	}

	// Marshal it
	data, err := env.MarshalBinary()
	if err != nil {
		g.logger.Error("Failed to marshal internal transaction", "error", err)
		return
	}

	// Send it
	g.logger.Debug("Sending internal txn", "txid", logging.AsHex(env.GetTxHash()), "type", body.GetType())
	g.dispatcher.BroadcastTxAsyncLocal(context.TODO(), data)
}
