// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"fmt"
	"math/big"
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// Messages with a field left out are refused (#4572): the named cases, then
// every message type with every field left out. Every case goes through validation and, past it, through a
// block, which is where a proposer can put what validation refused. A panic
// on a simulator goroutine fails the run outright; noPanic reports one on the
// test's own.

// signedAnchor is a block anchor the source partition's validator signed, so
// that validation gets past the signature to the body.
func signedAnchor(t *testing.T, sim *Sim, source string, dest *url.URL, body TransactionBody) *messaging.BlockAnchor {
	t.Helper()
	seq := &messaging.SequencedMessage{
		Message: &messaging.TransactionMessage{Transaction: &Transaction{
			Header: TransactionHeader{Principal: dest.JoinPath(AnchorPool)},
			Body:   body,
		}},
		Source:      PartitionUrl(source),
		Destination: dest,
		Number:      1,
	}
	h := seq.Hash()
	sig, err := new(signing.Builder).
		SetType(SignatureTypeED25519).
		SetPrivateKey(sim.S.SignWithNode(source, 0).Key()).
		SetUrl(PartitionUrl(source).JoinPath(Network)).
		SetVersion(1).
		SetTimestamp(1).
		Sign(h[:])
	require.NoError(t, err)
	return &messaging.BlockAnchor{Anchor: seq, Signature: sig.(KeySignature)}
}

func wrapSeq(msg messaging.Message) *messaging.SequencedMessage {
	return &messaging.SequencedMessage{Message: msg, Source: PartitionUrl("BVN0"), Destination: DnUrl(), Number: 1}
}

func noPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	var r any
	var stack []byte
	func() {
		defer func() {
			r = recover()
			if r != nil {
				stack = debug.Stack()
			}
		}()
		fn()
	}()
	require.Nil(t, r, "%s panicked: %v\n%s", what, r, stack)
}

func TestMalformedMessagesAreRefused(t *testing.T) {
	inner := func() *messaging.SequencedMessage {
		return &messaging.SequencedMessage{
			Message: &messaging.TransactionMessage{Transaction: &Transaction{
				Header: TransactionHeader{Principal: AcmeUrl()},
				Body:   &SyntheticDepositTokens{Token: AcmeUrl(), Amount: *big.NewInt(1)},
			}},
			Source:      PartitionUrl("BVN0"),
			Destination: DnUrl(),
			Number:      1,
		}
	}

	cases := []struct {
		Name      string
		Partition string
		Message   func(*Sim) messaging.Message
	}{
		{"directory anchor without a source", "BVN0", func(sim *Sim) messaging.Message {
			return signedAnchor(t, sim, Directory, PartitionUrl("BVN0"), &DirectoryAnchor{})
		}},
		{"partition anchor without a source", Directory, func(sim *Sim) messaging.Message {
			return signedAnchor(t, sim, "BVN0", DnUrl(), &BlockValidatorAnchor{})
		}},
		{"deprecated synthetic message without a proof", Directory, func(*Sim) messaging.Message {
			return &messaging.BadSyntheticMessage{Message: inner()}
		}},
		{"deprecated synthetic message without a receipt", Directory, func(*Sim) messaging.Message {
			return &messaging.BadSyntheticMessage{Message: inner(), Proof: &AnnotatedReceipt{}}
		}},
		{"deprecated synthetic message with a short receipt anchor", Directory, func(*Sim) messaging.Message {
			return &messaging.BadSyntheticMessage{Message: inner(), Proof: &AnnotatedReceipt{
				Receipt: &merkle.Receipt{Anchor: []byte{1}},
			}}
		}},
		{"deprecated synthetic message without a message", Directory, func(*Sim) messaging.Message {
			return &messaging.BadSyntheticMessage{}
		}},
		{"synthetic message without a message", Directory, func(*Sim) messaging.Message {
			return &messaging.SyntheticMessage{}
		}},
		{"deprecated synthetic message with a receipt list and no receipt", Directory, func(*Sim) messaging.Message {
			return &messaging.BadSyntheticMessage{Message: inner(), Proof: &AnnotatedReceipt{
				ReceiptList: &merkle.ReceiptList{},
			}}
		}},
		{"anchor of a transaction without a body", "BVN0", func(sim *Sim) messaging.Message {
			return signedAnchor(t, sim, Directory, PartitionUrl("BVN0"), nil)
		}},
		{"anchor of an empty sequence", "BVN0", func(*Sim) messaging.Message {
			return &messaging.BlockAnchor{Anchor: &messaging.SequencedMessage{}}
		}},
		{"sequenced message without a message", Directory, func(*Sim) messaging.Message {
			return &messaging.SequencedMessage{Source: PartitionUrl("BVN0"), Destination: DnUrl(), Number: 1}
		}},
		{"sequenced transaction message without a transaction", Directory, func(*Sim) messaging.Message {
			return wrapSeq(&messaging.TransactionMessage{})
		}},
		{"sequenced transaction without a body", Directory, func(*Sim) messaging.Message {
			return wrapSeq(&messaging.TransactionMessage{Transaction: &Transaction{Header: TransactionHeader{Principal: AcmeUrl()}}})
		}},
		{"sequenced signature message without a signature", Directory, func(*Sim) messaging.Message {
			return wrapSeq(&messaging.SignatureMessage{})
		}},
		{"sequenced credit payment without a transaction ID", Directory, func(*Sim) messaging.Message {
			return wrapSeq(&messaging.CreditPayment{})
		}},
		{"synthetic message of an empty sequence", Directory, func(*Sim) messaging.Message {
			return &messaging.SyntheticMessage{Message: &messaging.SequencedMessage{}}
		}},
		{"synthetic message of an empty synthetic message", Directory, func(*Sim) messaging.Message {
			return &messaging.SyntheticMessage{Message: &messaging.SyntheticMessage{}}
		}},
		{"synthetic message of a transaction without a body", Directory, func(*Sim) messaging.Message {
			return &messaging.SyntheticMessage{Message: wrapSeq(&messaging.TransactionMessage{Transaction: &Transaction{Header: TransactionHeader{Principal: AcmeUrl()}}})}
		}},
		{"deprecated synthetic message of a transaction without a body", Directory, func(*Sim) messaging.Message {
			return &messaging.BadSyntheticMessage{Message: wrapSeq(&messaging.TransactionMessage{Transaction: &Transaction{Header: TransactionHeader{Principal: AcmeUrl()}}})}
		}},
	}

	// Every message type with every field left out
	for typ := messaging.MessageType(1); typ < 256; typ++ {
		if _, err := messaging.NewMessage(typ); err != nil {
			continue
		}
		for _, part := range []string{"BVN0", Directory} {
			typ, part := typ, part
			cases = append(cases, struct {
				Name      string
				Partition string
				Message   func(*Sim) messaging.Message
			}{fmt.Sprintf("empty %v on %s", typ, part), part, func(*Sim) messaging.Message {
				msg, _ := messaging.NewMessage(typ)
				return msg
			}})
		}
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			sim := NewSim(t,
				simulator.SimpleNetwork(t.Name(), 1, 1),
				simulator.Genesis(GenesisTime),
			)
			msg := c.Message(sim)

			// Validation refuses it
			noPanic(t, "validation", func() {
				statuses, err := sim.SubmitTo(c.Partition, &messaging.Envelope{Messages: []messaging.Message{msg}})
				if err != nil {
					return // Refused before validation
				}
				// Refused, or for a synthetic message without a proof,
				// pending one; never accepted
				require.Len(t, statuses, 1)
				require.NotEqual(t, errors.OK, statuses[0].Code)
			})

			// A block that carries it anyway executes
			injected := false
			sim.S.SetBlockHook(c.Partition, func(_ execute.BlockParams, env []*messaging.Envelope) ([]*messaging.Envelope, bool) {
				injected = true
				return append(env, &messaging.Envelope{Messages: []messaging.Message{c.Message(sim)}}), false
			})
			noPanic(t, "block", func() {
				for i := 0; i < 3; i++ {
					// An envelope refused whole fails the simulator's block;
					// the DAG-BFT bridge logs it and goes on. Either is a
					// refusal, not a panic.
					err := sim.S.Step()
					if err != nil {
						require.ErrorContains(t, err, "deliver envelope")
						return
					}
				}
			})
			require.True(t, injected, fmt.Sprintf("the %s block hook never ran", c.Partition))
		})
	}
}
