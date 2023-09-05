// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package healing

import (
	"context"
	"encoding/hex"
	"strings"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/exp/slog"
)

func HealAnchor(ctx context.Context,
	C1 api.Submitter, C2 *message.Client, net *NetworkInfo,
	srcUrl, dstUrl *url.URL, seqNum uint64,
	theAnchorTxn *protocol.Transaction, sigSets []*api.SignatureSetRecord,
	pretend bool,
) error {
	srcId, ok := protocol.ParsePartitionUrl(srcUrl)
	if !ok {
		panic("not a partition: " + srcUrl.String())
	}

	dstId, ok := protocol.ParsePartitionUrl(dstUrl)
	if !ok {
		panic("not a partition: " + dstUrl.String())
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
		Oracle:          net.Status.Oracle,
		Globals:         net.Status.Globals,
		Network:         net.Status.Network,
		Routing:         net.Status.Routing,
		ExecutorVersion: net.Status.ExecutorVersion,
	}
	threshold := g.ValidatorThreshold(srcId)

	lkv := []any{
		"source", srcId,
		"destination", dstId,
		"sequence-number", seqNum,
		"want", threshold,
		"have", len(signed),
	}
	if theAnchorTxn != nil {
		lkv = append(lkv,
			"txid", theAnchorTxn.ID(),
		)
	}
	slog.InfoCtx(ctx, "Healing anchor", lkv...)

	if len(signed) >= int(threshold) {
		slog.InfoCtx(ctx, "Sufficient signatures have been received")
		return nil
	}

	seq := &messaging.SequencedMessage{
		Source:      srcUrl,
		Destination: dstUrl,
		Number:      seqNum,
	}
	if theAnchorTxn != nil {
		seq.Message = &messaging.TransactionMessage{
			Transaction: theAnchorTxn,
		}
	}

	// Get a signature from each node that hasn't signed
	type fromPeer struct {
		gotPartSig bool
		signatures []protocol.Signature
	}

	type qPeer struct {
		peer peer.ID
		info *PeerInfo
	}

	peers := make(chan *qPeer, 10)
	sigs := make(chan *fromPeer, 10)

	process := func() {
		for {
			fp := new(fromPeer)
			defer func() { sigs <- fp }()
			func() {
				qp := <-peers
				peer := qp.peer
				info := qp.info

				if signed[info.Key] {
					return
				}

				ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
				defer cancel()

				slog.InfoCtx(ctx, "Querying node for its signature", "id", peer)
				res, err := C2.ForPeer(peer).Private().Sequence(ctx, srcUrl.JoinPath(protocol.AnchorPool), dstUrl, seqNum)
				if err != nil {
					slog.ErrorCtx(ctx, "Query failed", "error", err)
					return
				}

				myTxn, ok := res.Message.(*messaging.TransactionMessage)
				if !ok {
					slog.ErrorCtx(ctx, "Node gave us an anchor that is not a transaction", "id", info, "type", res.Message.Type())
					return
				}
				if theAnchorTxn == nil {
					theAnchorTxn = myTxn.Transaction
					seq.Message = &messaging.TransactionMessage{
						Transaction: theAnchorTxn,
					}
				} else if !myTxn.Transaction.Equal(theAnchorTxn) {
					slog.ErrorCtx(ctx, "Node gave us an anchor with a different hash", "id", info,
						"expected", hex.EncodeToString(theAnchorTxn.GetHash()),
						"got", hex.EncodeToString(myTxn.Transaction.GetHash()))
					return
				}

				for _, sigs := range res.Signatures.Records {
					for _, sig := range sigs.Signatures.Records {
						msg, ok := sig.Message.(*messaging.SignatureMessage)
						if !ok {
							slog.ErrorCtx(ctx, "Node gave us a signature that is not a signature", "id", info, "type", sig.Message.Type())
							continue
						}

						if net.Status.ExecutorVersion.V2() {
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
								if fp.gotPartSig {
									continue
								}
								fp.gotPartSig = true

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

						fp.signatures = append(fp.signatures, msg.Signature)
					}
				}

			}()
		}
	}

	for i := 0; i < 10; i++ { // How many parallel processes
		go process() //          Start them up
	}

	var done atomic.Bool // We quit collecting when we have queued all the peers AND we have processed all sigs
	go func() {          // Start a go routine to grab all the peers and put them into a channel
		for peer, info := range net.Peers[strings.ToLower(srcId)] {
			qp := new(qPeer)
			qp.peer = peer
			qp.info = info
			peers <- qp
		}
		done.Store(true)
	}()

	var gotPartSig bool                 // This is the original bool that somewhere we have a Partial Signature
	var signatures []protocol.Signature // This collects all the signatures

	for !done.Load() && len(sigs) == 0 { // For every return value from the collection process
		sig := <-sigs                                      // We get a partial set of data
		gotPartSig = gotPartSig || sig.gotPartSig          // If any partial signature is found, we set the variable
		signatures = append(signatures, sig.signatures...) // And collect all the signatures
	}

	if pretend {
		b, err := theAnchorTxn.MarshalBinary()
		if err != nil {
			panic(err)
		}
		slog.InfoCtx(ctx, "Would have submitted anchor", "signatures", len(signatures), "source", srcId, "destination", dstId, "number", seqNum, "txn-size", len(b))
		return nil
	}

	// We should always have a partition signature, so there's only something to
	// sent if we have more than 1 signature
	if gotPartSig && len(signatures) <= 1 || !gotPartSig && len(signatures) == 0 {
		slog.InfoCtx(ctx, "Nothing to send")
		return nil
	}

	slog.InfoCtx(ctx, "Submitting signatures", "count", len(signatures))
	env := new(messaging.Envelope)
	if net.Status.ExecutorVersion.V2() {
		for i, sig := range signatures {
			seq := seq.Copy()
			if i > 0 {
				seq.Message = &messaging.TransactionMessage{
					Transaction: &protocol.Transaction{
						Body: &protocol.RemoteTransaction{
							Hash: theAnchorTxn.ID().Hash(),
						},
					},
				}
			}
			env.Messages = append(env.Messages, &messaging.BlockAnchor{
				Signature: sig.(protocol.KeySignature),
				Anchor:    seq,
			})
		}
	} else {
		env.Transaction = []*protocol.Transaction{theAnchorTxn}
		env.Signatures = signatures
	}

	// addr := api.ServiceTypeSubmit.AddressFor(dstId).Multiaddr()
	sub, err := C1.Submit(ctx, env, api.SubmitOptions{})
	if err != nil {
		slog.ErrorCtx(ctx, "Submission failed", "error", err)
	}
	for _, sub := range sub {
		if !sub.Success {
			slog.ErrorCtx(ctx, "Submission failed", "message", sub, "status", sub.Status)
		}
	}

	return nil
}
