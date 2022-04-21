package block

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	jrpc "github.com/tendermint/tendermint/rpc/jsonrpc/types"
	tm "github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const debugSendAnchor = false
const debugAnchorUpdates = false

type governor struct {
	ExecutorOptions
	logger     logging.OptionalLogger
	db         *database.Database
	dispatcher *dispatcher
	started    int32
	messages   chan interface{}
	done       chan struct{}
	sent       map[[32]byte]bool
}

type govStop struct{}

type govPing struct{}

type govDidCommit struct {
	mirrorAdi  bool
	block      Block
	ledger     *protocol.InternalLedger
	rootAnchor []byte
	rootHeight int64
}

func newGovernor(opts ExecutorOptions, db *database.Database) *governor {
	g := new(governor)
	g.ExecutorOptions = opts
	g.db = db
	g.messages = make(chan interface{})
	g.done = make(chan struct{})
	g.sent = map[[32]byte]bool{}
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

func (g *governor) DidCommit(batch *database.Batch, mirrorAdi bool, block *Block) error {
	g.logger.Debug("Block event", "type", "didCommit", "height", block.Index, "time", block.Time)

	if !block.IsLeader {
		// Nothing to do if we're not the leader
		return nil
	}

	msg := govDidCommit{
		mirrorAdi: mirrorAdi,
		block:     *block,
	}

	ledger := batch.Account(g.Network.Ledger())
	err := ledger.GetStateAs(&msg.ledger)
	if err != nil {
		return err
	}

	rootChain, err := ledger.ReadChain(protocol.MinorRootChain)
	if err != nil {
		return err
	}
	msg.rootAnchor = rootChain.Anchor()
	msg.rootHeight = rootChain.Height()

	select {
	case g.messages <- msg:
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
		switch msg := msg.(type) {
		case govStop:
			return

		case govDidCommit:
			g.runDidCommit(&msg)
		}
	}
}

func (g *governor) runDidCommit(msg *govDidCommit) {
	// The governor must be read-only, so we must not commit the
	// database transaction or the state cache. If the governor makes
	// ANY changes, the system will no longer be deterministic.
	batch := g.db.Begin(false)
	defer batch.Discard()

	// TODO This will hit the database with a lot of queries, maybe we shouldn't do this
	producedCount := countExceptAnchors2(msg.block.State.ProducedTxns)
	unsignedCount := countExceptAnchors(batch, msg.ledger.Synthetic.Unsigned)
	unsentCount := countExceptAnchors(batch, msg.ledger.Synthetic.Unsent)

	g.logger.Info("Did commit",
		"height", msg.block.Index,
		"time", msg.block.Time,
		"mirror", msg.mirrorAdi,
		"updated", len(msg.block.State.ChainUpdates.Entries),
		"produced", producedCount,
		"unsigned", unsignedCount,
		"unsent", unsentCount,
	)

	// Mirror the subnet's ADI
	if msg.mirrorAdi {
		g.sendMirror(batch)
	}

	// Create an anchor for the block
	g.sendAnchor(batch, msg, producedCount)

	// Sign and send produced synthetic transactions
	g.signTransactions(batch, msg.ledger)
	g.sendTransactions(batch, msg)

	// Dispatch transactions asynchronously
	errs := g.dispatcher.Send(context.Background())
	go func() {
		for err := range errs {
			g.checkDispatchError(err)
		}
	}()
}

// checkDispatchError returns nil if the error can be ignored.
func (g *governor) checkDispatchError(err error) {
	if err == nil {
		return
	}

	// TODO This may be unnecessary once this issue is fixed:
	// https://github.com/tendermint/tendermint/issues/7185.

	// Is the error "tx already exists in cache"?
	if err.Error() == tm.ErrTxInCache.Error() {
		return
	}

	// Or RPC error "tx already exists in cache"?
	var rpcErr1 *jrpc.RPCError
	if errors.As(err, &rpcErr1) && *rpcErr1 == *errTxInCache1 {
		return
	}

	var rpcErr2 jsonrpc2.Error
	if errors.As(err, &rpcErr2) && rpcErr2 == errTxInCache2 {
		return
	}

	var protoErr *protocol.Error
	if errors.As(err, &protoErr) {
		// Without this, the simulator generates tons of errors. Why?
		if protoErr.Code == protocol.ErrorCodeAlreadyDelivered {
			return
		}
	}

	// It's a real error
	g.logger.Error("Failed to dispatch transactions", "error", err)
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
			g.logger.Error("Failed to load pending transaction", "txid", logging.AsHex(txid).Slice(0, 4), "error", err)
			continue
		}

		typ := tx.Transaction.Body.Type()
		if typ != protocol.TransactionTypeSyntheticAnchor {
			g.logger.Debug("Signing synth txn", "txid", logging.AsHex(txid).Slice(0, 4), "type", typ)
		}

		// Sign it
		ed, err := new(signing.Builder).
			SetType(protocol.SignatureTypeED25519).
			SetPrivateKey(g.Key).
			SetKeyPageUrl(g.Network.ValidatorBook(), 0).
			SetVersion(1).
			SetTimestamp(1).
			Sign(txid[:])
		if err != nil {
			g.logger.Error("Failed to sign pending transaction", "txid", logging.AsHex(txid).Slice(0, 4), "error", err)
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

func (g *governor) sendTransactions(batch *database.Batch, msg *govDidCommit) {
	if len(msg.ledger.Synthetic.Unsent) == 0 {
		return
	}

	body := new(protocol.InternalTransactionsSent)
	body.Transactions = make([][32]byte, 0, len(msg.ledger.Synthetic.Unsent))

	// For each unsent synthetic transaction
	for _, id := range msg.ledger.Synthetic.Unsent {
		// Load state
		obj := batch.Transaction(id[:])
		pending, err := obj.GetState()
		if err != nil {
			g.logger.Error("Failed to load pending transaction", "txid", logging.AsHex(id).Slice(0, 4), "error", err)
			continue
		}

		// Load status
		status, err := obj.GetStatus()
		if err != nil {
			g.logger.Error("Failed to load pending transaction status", "txid", logging.AsHex(id).Slice(0, 4), "error", err)
			return
		}

		// Load signatures
		signatures, err := getAllSignatures(batch, obj, status, pending.Transaction.Header.Initiator[:])
		if err != nil {
			g.logger.Error("Failed to load pending transaction signatures", "txid", logging.AsHex(id).Slice(0, 4), "error", err)
			return
		}

		if len(signatures) == 0 {
			g.logger.Error("Transaction has no signatures!", "txid", logging.AsHex(id).Slice(0, 4))
			continue
		}

		// Convert it back to a transaction
		env := new(protocol.Envelope)
		env.Transaction = []*protocol.Transaction{pending.Transaction}
		env.Signatures = signatures

		typ := pending.Transaction.Type()
		txid32 := *(*[32]byte)(pending.Transaction.GetHash())
		if g.sent[txid32] {
			g.logger.Info("Resending synth txn", "origin", pending.Transaction.Header.Principal, "txid", logging.AsHex(pending.Transaction.GetHash()).Slice(0, 4), "type", typ, "block", msg.block.Index)
		} else {
			if debugSendAnchor || typ != protocol.TransactionTypeSyntheticAnchor {
				g.logger.Debug("Sending synth txn",
					"origin", pending.Transaction.Header.Principal,
					"txn-hash", logging.AsHex(pending.Transaction.GetHash()).Slice(0, 4),
					"type", typ,
					"block", msg.block.Index)
			}
			g.sent[txid32] = true
		}

		// Send it
		err = g.dispatcher.BroadcastTx(context.Background(), pending.Transaction.Header.Principal, env)
		if err != nil {
			g.logger.Error("Failed to dispatch transaction", "txid", logging.AsHex(id).Slice(0, 4), "error", err)
			continue
		}
		body.Transactions = append(body.Transactions, id)
	}

	g.sendInternal(batch, body)
}

func (g *governor) sendAnchor(batch *database.Batch, msg *govDidCommit, synthCountExceptAnchors int) {
	// Don't create an anchor transaction if no records were updated and no
	// synthetic transactions (other than synthetic anchors) were produced
	if len(msg.block.State.ChainUpdates.Entries) == 0 && synthCountExceptAnchors == 0 {
		return
	}

	if msg.block.Anchor == nil {
		panic("TODO When is it OK for the anchor to be nil?")
	}

	kv := []interface{}{"root", logging.AsHex(msg.block.Anchor.RootAnchor).Slice(0, 4)}
	if debugAnchorUpdates {
		for i, c := range msg.block.State.ChainUpdates.Entries {
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
	}
	g.logger.Debug("Creating anchor txn", kv...)

	txns := new(protocol.InternalSendTransactions)
	switch g.Network.Type {
	case config.Directory:
		// Send anchors from DN to all BVNs
		bvnNames := g.Network.GetBvnNames()
		txns.Transactions = make([]protocol.SendTransaction, len(bvnNames))
		for i, bvn := range bvnNames {
			txns.Transactions[i] = protocol.SendTransaction{
				Recipient: protocol.SubnetUrl(bvn).JoinPath(protocol.AnchorPool),
				Payload:   msg.block.Anchor,
			}
		}

	case config.BlockValidator:
		// Send anchor from BVN to DN
		txns.Transactions = []protocol.SendTransaction{{
			Recipient: protocol.DnUrl().JoinPath(protocol.AnchorPool),
			Payload:   msg.block.Anchor,
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

	md, err := loadDirectoryMetadata(batch, nodeUrl)
	if err != nil {
		g.logger.Error("Failed to load directory", "error", err, "url", nodeUrl)
		return
	}

	for i := uint64(0); i < md.Count; i++ {
		s, err := loadDirectoryEntry(batch, nodeUrl, i)
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
				Recipient: protocol.SubnetUrl(bvn),
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

func (g *governor) sendInternal(batch *database.Batch, body protocol.TransactionBody) {
	// Construct the signature transaction
	var ledgerState *protocol.InternalLedger
	err := batch.Account(g.Network.Ledger()).GetStateAs(&ledgerState)
	if err != nil {
		// If we can't load the ledger, the node is fubared
		panic(fmt.Errorf("failed to load the ledger: %v", err))
	}

	txn := new(protocol.Transaction)
	txn.Header.Principal = g.Network.Ledger()
	txn.Body = body
	env := new(protocol.Envelope)
	env.Transaction = []*protocol.Transaction{txn}

	// Sign it
	ed, err := new(signing.Builder).
		SetType(protocol.SignatureTypeED25519).
		SetPrivateKey(g.Key).
		SetUrl(g.Network.ValidatorPage(0)).
		SetVersion(1).
		SetTimestamp(uint64(ledgerState.Index) + 1).
		Initiate(txn)
	if err != nil {
		g.logger.Error("Failed to sign internal transaction", "error", err)
		return
	}
	env.Signatures = append(env.Signatures, ed)

	// Send it
	g.logger.Debug("Sending internal txn", "txid", logging.AsHex(txn.GetHash()).Slice(0, 4), "type", body.Type())
	err = g.dispatcher.BroadcastTxLocal(context.TODO(), env)
	if err != nil {
		g.logger.Error("Failed to send internal transaction", "error", err)
		return
	}
}
