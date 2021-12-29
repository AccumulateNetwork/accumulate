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
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/tendermint/tendermint/libs/log"
)

type governor struct {
	ExecutorOptions
	logger     log.Logger
	dispatcher *dispatcher
	started    int32
	messages   chan interface{}
	done       chan struct{}
}

type govStop struct{}

type govDidBeginBlock struct {
	isLeader bool
}

type govDidCommit struct {
	isLeader       bool
	height         int64
	time           time.Time
	rootAnchor     []byte
	synthAnchor    []byte
	recordsChanged [][32]byte
}

func newGovernor(opts ExecutorOptions) (*governor, error) {
	g := new(governor)
	g.ExecutorOptions = opts
	g.messages = make(chan interface{})
	g.done = make(chan struct{})

	if opts.Logger != nil {
		g.logger = opts.Logger.With("module", "governor")
	}

	var err error
	g.dispatcher, err = newDispatcher(opts)
	if err != nil {
		return nil, err
	}

	return g, nil
}

func (g *governor) logDebug(msg string, keyVals ...interface{}) {
	if g.logger != nil {
		g.logger.Debug(msg, keyVals...)
	}
}

func (g *governor) logInfo(msg string, keyVals ...interface{}) {
	if g.logger != nil {
		g.logger.Info(msg, keyVals...)
	}
}

func (g *governor) logError(msg string, keyVals ...interface{}) {
	if g.logger != nil {
		g.logger.Error(msg, keyVals...)
	}
}

func (g *governor) Start() error {
	if !atomic.CompareAndSwapInt32(&g.started, 0, 1) {
		return errors.New("already started")
	}

	go g.run()
	return nil
}

func (g *governor) DidBeginBlock(isLeader bool) {
	select {
	case g.messages <- govDidBeginBlock{
		isLeader: isLeader,
	}:
	case <-g.done:
	}
}

func (g *governor) DidCommit(batch *database.Batch, isLeader bool, height int64, time time.Time) error {
	root := batch.Record(g.Network.NodeUrl().JoinPath(protocol.MinorRoot))
	rootState := state.NewAnchor()
	err := root.GetStateAs(rootState)
	if err != nil {
		return err
	}

	rootChain, err := root.Chain(protocol.Main)
	if err != nil {
		return err
	}

	synthUrl := g.Network.NodeUrl().JoinPath(protocol.Synthetic)
	synthChain, err := batch.Record(synthUrl).Chain(protocol.Main)
	if err != nil {
		return err
	}

	select {
	case g.messages <- govDidCommit{
		isLeader:       isLeader,
		height:         height,
		time:           time,
		rootAnchor:     rootChain.Anchor(),
		synthAnchor:    synthChain.Anchor(),
		recordsChanged: rootState.Chains,
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
			ledger := batch.Record(g.Network.NodeUrl().JoinPath(protocol.Ledger))
			ledgerState := new(protocol.InternalLedger)
			err := ledger.GetStateAs(ledgerState)
			if err != nil {
				// If we can't load the ledger, the node is fubared
				panic(fmt.Errorf("failed to load the ledger: %v", err))
			}

			if msg.isLeader && len(ledgerState.Synthetic.Unsigned) > 0 {
				tx := g.signTransactions(batch, ledgerState)
				g.sendInternal(tx)
			}

			if len(ledgerState.Synthetic.Unsent) > 0 {
				tx := g.sendTransactions(batch, ledgerState)
				if msg.isLeader {
					g.sendInternal(tx)
				}
			}

		case govDidCommit:
			if msg.isLeader {
				tx := g.sendAnchor(batch, &msg)
				g.sendInternal(tx)
			}
		}

		err := g.dispatcher.Send(context.Background())
		if err != nil {
			g.logger.Error("Failed to dispatch transactions", "error", err)
		}
	}
}

func (g *governor) sendInternal(body protocol.TransactionPayload) {
	// Construct the signature transaction
	tx, err := g.buildSynthTxn(g.Network.NodeUrl().JoinPath(protocol.Ledger), body, nil)
	if err != nil {
		g.logError("Failed to build internal transaction", "error", err)
	}

	// Sign it
	ed := new(transactions.ED25519Sig)
	tx.Signature = append(tx.Signature, ed)
	ed.PublicKey = g.Key[32:]
	err = ed.Sign(tx.SigInfo.Nonce, g.Key, tx.TransactionHash())
	if err != nil {
		g.logError("Failed to sign internal transaction", "error", err)
	}

	// Marshal it
	data, err := tx.Marshal()
	if err != nil {
		g.logError("Failed to marshal internal transaction", "error", err)
	}

	// Send it
	g.dispatcher.BroadcastTxAsyncLocal(context.TODO(), data)
}

func (g *governor) signTransactions(batch *database.Batch, ledger *protocol.InternalLedger) protocol.TransactionPayload {
	body := new(protocol.InternalTransactionsSigned)
	body.Transactions = make([]protocol.TransactionSignature, 0, len(ledger.Synthetic.Unsigned))

	// For each unsigned synthetic transaction
	for _, txid := range ledger.Synthetic.Unsigned {
		g.logDebug("Signing synth txn", "txid", logging.AsHex(txid))

		// Load it
		tx, err := batch.Transaction(txid[:]).GetState()
		if err != nil {
			g.logError("Failed to load pending transaction", "txid", logging.AsHex(txid), "error", err)
			continue
		}

		// Sign it
		ed := new(transactions.ED25519Sig)
		ed.PublicKey = g.Key[32:]
		err = ed.Sign(tx.SigInfo.Nonce, g.Key, txid[:])
		if err != nil {
			g.logError("Failed to sign pending transaction", "txid", logging.AsHex(txid), "error", err)
			continue
		}

		// Add it to the list
		var sig protocol.TransactionSignature
		sig.Transaction = txid
		sig.Signature = ed
		body.Transactions = append(body.Transactions, sig)
	}

	return body
}

func (g *governor) sendTransactions(batch *database.Batch, ledger *protocol.InternalLedger) protocol.TransactionPayload {
	body := new(protocol.InternalTransactionsSent)
	body.Transactions = make([][32]byte, 0, len(ledger.Synthetic.Unsent))

	// For each unsent synthetic transaction
	for _, id := range ledger.Synthetic.Unsent {
		// Load it
		pending, _, signatures, err := batch.Transaction(id[:]).Get()
		if err != nil {
			g.logError("Failed to load pending transaction", "txid", logging.AsHex(id), "error", err)
			continue
		}

		if len(signatures) == 0 {
			g.logError("Transaction has no signatures!", "txid", logging.AsHex(id))
			continue
		}

		// Convert it back to a transaction
		tx := pending.Restore()
		tx.Signature = signatures

		// Marshal it
		raw, err := tx.Marshal()
		if err != nil {
			g.logError("Failed to marshal pending transaction", "txid", logging.AsHex(id), "error", err)
			continue
		}

		// Parse the URL
		u, err := url.Parse(tx.SigInfo.URL)
		if err != nil {
			g.logError("Invalid pending transaction URL", "txid", logging.AsHex(id), "error", err, "url", tx.SigInfo.URL)
			continue
		}

		// Send it
		g.logDebug("Sending synth txn", "actor", u.String(), "txid", logging.AsHex(tx.TransactionHash()))
		g.dispatcher.BroadcastTxAsync(context.Background(), u, raw)
		body.Transactions = append(body.Transactions, id)
	}

	return body
}

func (g *governor) sendAnchor(batch *database.Batch, msg *govDidCommit) protocol.TransactionPayload {
	body := new(protocol.SyntheticAnchor)
	body.Source = g.Network.NodeUrl().String()
	body.Index = msg.height
	body.Timestamp = msg.time
	copy(body.Root[:], batch.RootHash())
	body.Chains = msg.recordsChanged
	copy(body.ChainAnchor[:], msg.rootAnchor)
	copy(body.SynthTxnAnchor[:], msg.synthAnchor)

	g.logDebug("Creating anchor txn", "root", logging.AsHex(body.Root), "chains", logging.AsHex(body.ChainAnchor), "synth", logging.AsHex(body.SynthTxnAnchor))

	txns := new(protocol.InternalSendTransactions)
	switch g.Network.Type {
	case config.Directory:
		// Send anchors from DN to all BVNs
		txns.Transactions = make([]protocol.SendTransaction, len(g.Network.BvnNames))
		for i, bvn := range g.Network.BvnNames {
			txns.Transactions[i] = protocol.SendTransaction{
				Payload:   protocol.WrappedTxPayload{TransactionPayload: body},
				Recipient: protocol.BvnUrl(bvn).String(),
			}
		}

	case config.BlockValidator:
		// Send anchor from BVN to DN
		txns.Transactions = []protocol.SendTransaction{{
			Payload:   protocol.WrappedTxPayload{TransactionPayload: body},
			Recipient: protocol.DnUrl().String(),
		}}
	}
	return txns
}
