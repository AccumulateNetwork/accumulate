// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package healing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/exp/slog"
)

type HealAnchorArgs struct {
	Client    message.AddressedClient
	Querier   api.Querier
	Submitter api.Submitter
	NetInfo   *NetworkInfo
	Known     map[[32]byte]*protocol.Transaction
	Pretend   bool
	Wait      bool
}

func HealAnchor(ctx context.Context, args HealAnchorArgs, si SequencedInfo) error {
	srcUrl := protocol.PartitionUrl(si.Source)
	dstUrl := protocol.PartitionUrl(si.Destination)

	if args.Querier == nil {
		args.Querier = args.Client
	}
	if args.Submitter == nil {
		args.Submitter = args.Client
	}

	// If the message ID is not known, resolve it
	var theAnchorTxn *protocol.Transaction
	if si.ID == nil {
		r, err := ResolveSequenced[*messaging.TransactionMessage](ctx, args.Client, args.NetInfo, si.Source, si.Destination, si.Number, true)
		if err != nil {
			return err
		}
		si.ID = r.ID
		theAnchorTxn = r.Message.Transaction
	}

	// Fetch the transaction and signatures
	var sigSets []*api.SignatureSetRecord
	Q := api.Querier2{Querier: args.Querier}
	res, err := Q.QueryMessage(ctx, si.ID, nil)
	switch {
	case err == nil:
		if res.Status.Delivered() {
			slog.InfoCtx(ctx, "Anchor has been delivered", "id", si.ID, "source", si.Source, "destination", si.Destination, "number", si.Number)
			return errors.Delivered
		}
		switch msg := res.Message.(type) {
		case *messaging.SequencedMessage:
			txn, ok := msg.Message.(*messaging.TransactionMessage)
			if !ok {
				return errors.InternalError.WithFormat("expected %v, got %v", messaging.MessageTypeTransaction, msg.Message.Type())
			}
			theAnchorTxn = txn.Transaction
		case *messaging.TransactionMessage:
			theAnchorTxn = msg.Transaction
		default:
			return errors.InternalError.WithFormat("expected %v, got %v", messaging.MessageTypeSequenced, res.Message.Type())
		}

		sigSets = res.Signatures.Records

	case !errors.Is(err, errors.NotFound):
		return err

	case theAnchorTxn == nil:
		var ok bool
		theAnchorTxn, ok = args.Known[si.ID.Hash()]
		if !ok {
			return err
		}
	}

	// Mark which validators have signed
	signed := map[[32]byte]bool{}
	for _, sigs := range sigSets {
		for _, sig := range sigs.Signatures.Records {
			msg, ok := sig.Message.(*messaging.BlockAnchor)
			if !ok {
				continue
			}
			k := msg.Signature.GetPublicKey()
			slog.DebugCtx(ctx, "Anchor has been signed by", "validator", hex.EncodeToString(k[:4]))
			signed[*(*[32]byte)(k)] = true
		}
	}

	g := &network.GlobalValues{
		Oracle:          args.NetInfo.Status.Oracle,
		Globals:         args.NetInfo.Status.Globals,
		Network:         args.NetInfo.Status.Network,
		Routing:         args.NetInfo.Status.Routing,
		ExecutorVersion: args.NetInfo.Status.ExecutorVersion,
	}
	threshold := g.ValidatorThreshold(si.Source)

	slog.InfoCtx(ctx, "Healing anchor",
		"source", si.Source,
		"destination", si.Destination,
		"sequence-number", si.Number,
		"want", threshold,
		"have", len(signed),
		"txid", theAnchorTxn.ID())

	if len(signed) >= int(threshold) {
		slog.InfoCtx(ctx, "Sufficient signatures have been received")
		return errors.Delivered
	}

	seq := &messaging.SequencedMessage{
		Source:      srcUrl,
		Destination: dstUrl,
		Number:      si.Number,
	}
	if theAnchorTxn != nil {
		seq.Message = &messaging.TransactionMessage{
			Transaction: theAnchorTxn,
		}
	}

	// Get a signature from each node that hasn't signed
	var gotPartSig bool
	var signatures []protocol.Signature
	for peer, info := range args.NetInfo.Peers[strings.ToLower(si.Source)] {
		if signed[info.Key] {
			continue
		}

		addr := multiaddr.StringCast("/p2p/" + peer.String())
		if len(info.Addresses) > 0 {
			addr = info.Addresses[0].Encapsulate(addr)
		}

		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		slog.InfoCtx(ctx, "Querying node for its signature", "id", peer)
		res, err := args.Client.ForAddress(addr).Private().Sequence(ctx, srcUrl.JoinPath(protocol.AnchorPool), dstUrl, si.Number, private.SequenceOptions{})
		if err != nil {
			slog.ErrorCtx(ctx, "Query failed", "error", err)
			continue
		}

		myTxn, ok := res.Message.(*messaging.TransactionMessage)
		if !ok {
			slog.ErrorCtx(ctx, "Node gave us an anchor that is not a transaction", "id", info, "type", res.Message.Type())
			continue
		}
		if theAnchorTxn == nil {
			theAnchorTxn = myTxn.Transaction
			seq.Message = &messaging.TransactionMessage{
				Transaction: theAnchorTxn,
			}
		} else if !protocol.EqualTransactionBody(myTxn.Transaction.Body, theAnchorTxn.Body) {
			slog.ErrorCtx(ctx, "Node gave us an anchor with a different hash", "id", info,
				"expected", hex.EncodeToString(theAnchorTxn.GetHash()),
				"got", hex.EncodeToString(myTxn.Transaction.GetHash()))
			continue
		}

		for _, sigs := range res.Signatures.Records {
			for _, sig := range sigs.Signatures.Records {
				msg, ok := sig.Message.(*messaging.SignatureMessage)
				if !ok {
					slog.ErrorCtx(ctx, "Node gave us a signature that is not a signature", "id", info, "type", sig.Message.Type())
					continue
				}

				if args.NetInfo.Status.ExecutorVersion.V2Enabled() {
					sig, ok := msg.Signature.(protocol.KeySignature)
					if !ok {
						slog.ErrorCtx(ctx, "Node gave us a signature that is not a key signature", "id", info, "type", sig.Type())
						continue
					}

					// Filter out bad signatures
					h := seq.Hash()
					if !sig.Verify(nil, h[:]) {
						slog.ErrorCtx(ctx, "Node gave us an invalid signature", "id", info)
						continue
					}

				} else {
					switch sig := msg.Signature.(type) {
					case *protocol.PartitionSignature:
						// We only want one partition signature
						if gotPartSig {
							continue
						}
						gotPartSig = true

					case protocol.UserSignature:
						// Filter out bad signatures
						if !sig.Verify(nil, theAnchorTxn.GetHash()) {
							slog.ErrorCtx(ctx, "Node gave us an invalid signature", "id", info)
							continue
						}

					default:
						slog.ErrorCtx(ctx, "Node gave us a signature that is not a user signature", "id", info, "type", sig.Type())
						continue
					}
				}

				signatures = append(signatures, msg.Signature)
			}
		}
	}

	if args.Pretend {
		b, err := theAnchorTxn.MarshalBinary()
		if err != nil {
			panic(err)
		}
		slog.InfoCtx(ctx, "Would have submitted anchor", "signatures", len(signatures), "source", si.Source, "destination", si.Destination, "number", si.Number, "txn-size", len(b))
		return nil
	}

	// We should always have a partition signature, so there's only something to
	// sent if we have more than 1 signature
	if gotPartSig && len(signatures) <= 1 || !gotPartSig && len(signatures) == 0 {
		slog.InfoCtx(ctx, "Nothing to send")

	} else {
		slog.InfoCtx(ctx, "Submitting signatures", "count", len(signatures))

		submit := func(env *messaging.Envelope) {
			// addr := api.ServiceTypeSubmit.AddressFor(seq.Destination).Multiaddr()
			sub, err := args.Submitter.Submit(ctx, env, api.SubmitOptions{})
			if err != nil {
				b, e := env.MarshalBinary()
				if e == nil {
					h := sha256.Sum256(b)
					b = h[:]
				}
				slog.ErrorCtx(ctx, "Submission failed", "error", err, "id", env.Messages[0].ID(), "hash", logging.AsHex(b))
			}
			for _, sub := range sub {
				if sub.Success {
					slog.InfoCtx(ctx, "Submission succeeded", "id", sub.Status.TxID)
				} else {
					slog.ErrorCtx(ctx, "Submission failed", "message", sub, "status", sub.Status)
				}
			}
		}

		if args.NetInfo.Status.ExecutorVersion.V2Enabled() {
			for _, sig := range signatures {
				blk := &messaging.BlockAnchor{
					Signature: sig.(protocol.KeySignature),
					Anchor:    seq,
				}
				submit(&messaging.Envelope{Messages: []messaging.Message{blk}})
			}
		} else {
			env := new(messaging.Envelope)
			env.Transaction = []*protocol.Transaction{theAnchorTxn}
			env.Signatures = signatures
			submit(env)
		}
	}

	if args.Wait {
		return waitFor(ctx, Q.Querier, si.ID)
	}
	return nil
}

var ErrRetry = fmt.Errorf("retry")
