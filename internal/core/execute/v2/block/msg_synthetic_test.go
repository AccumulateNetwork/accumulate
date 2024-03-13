// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	errors.EnableLocationTracking()
}

func TestSyntheticAnchor(t *testing.T) {
	var x SyntheticMessage
	cases := []struct {
		Method   func(*database.Batch, *MessageContext) (*protocol.TransactionStatus, error)
		Anchor   bool
		Succeeds bool
		Message  string
	}{
		{x.Validate, false, true, "Validate succeeds if the anchor is unknown"},
		{x.Validate, true, true, "Validate succeeds if the anchor is known"},
		{x.Process, false, false, "Process fails if the anchor is unknown"},
		{x.Process, true, true, "Process succeeds if the anchor is known"},
	}

	// Set up the message
	seed := sha256.Sum256([]byte("validator"))
	key := ed25519.NewKeyFromSeed(seed[:])
	seq := &messaging.SequencedMessage{
		Message:     new(messaging.TransactionMessage),
		Source:      protocol.PartitionUrl("foo"),
		Destination: protocol.PartitionUrl("bar"),
		Number:      10,
	}
	hash := seq.Hash()
	syn := &messaging.BadSyntheticMessage{
		Message: seq,
		Proof: &protocol.AnnotatedReceipt{
			Anchor: &protocol.AnchorMetadata{
				Account: protocol.UnknownUrl(),
			},
			Receipt: &merkle.Receipt{
				Start:  hash[:],
				Anchor: hash[:],
			},
		},
	}
	sig := &protocol.ED25519Signature{
		PublicKey:       key[32:],
		Signer:          protocol.DnUrl().JoinPath(protocol.Network),
		SignerVersion:   1,
		TransactionHash: seq.Hash(),
	}
	protocol.SignED25519(sig, key, nil, hash[:])
	syn.Signature = sig

	// Set up the context
	globals := new(Globals)
	globals.Active.Network = &protocol.NetworkDefinition{
		Partitions: []*protocol.PartitionInfo{{
			ID:   "bar",
			Type: protocol.PartitionTypeBlockValidator,
		}},
		Validators: []*protocol.ValidatorInfo{{
			PublicKey:     key[32:],
			PublicKeyHash: sha256.Sum256(key[32:]),
			Partitions: []*protocol.ValidatorPartitionInfo{{
				ID:     "foo",
				Active: true,
			}},
		}},
	}

	block := &Block{
		Executor: &Executor{
			globals: globals,
			messageExecutors: map[messaging.MessageType]ExecutorFor[messaging.MessageType, *MessageContext]{
				messaging.MessageTypeSequenced: fakeExecutor{},
			},
		},
	}

	// Run the tests
	errMsg := fmt.Sprintf("invalid proof anchor: %x is not a known directory anchor", syn.Proof.Receipt.Anchor)
	for i, c := range cases {
		t.Run(fmt.Sprintf("Case %d", i+1), func(t *testing.T) {
			ctx := &MessageContext{
				message: syn,
				bundle:  &bundle{Block: block},
			}

			db := database.OpenInMemory(nil)
			db.SetObserver(execute.NewDatabaseObserver())
			batch := db.Begin(true)
			defer batch.Discard()

			if c.Anchor {
				err := batch.Account(ctx.Executor.Describe.AnchorPool()).
					AnchorChain(protocol.Directory).
					Root().
					Inner().
					AddEntry(syn.Proof.Receipt.Anchor, false)
				require.NoError(t, err)
			}

			status, err := c.Method(batch, ctx)
			require.NoError(t, err)
			if c.Succeeds {
				require.NoError(t, status.AsError(), c.Message)
			} else {
				require.EqualError(t, status.AsError(), errMsg, c.Message)
			}
		})
	}
}

type fakeExecutor struct{}

func (fakeExecutor) Process(*database.Batch, *MessageContext) (*protocol.TransactionStatus, error) {
	return nil, nil
}

func (fakeExecutor) Validate(*database.Batch, *MessageContext) (*protocol.TransactionStatus, error) {
	return nil, nil
}
