// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The routing key must actually be extracted from a real envelope.
//
// The first version of this read envelope.Signatures, which is the LEGACY
// field and is empty in practice — signatures travel as messages. Every
// submission therefore routed with an empty key, which is round-robin, which
// is the bug the routing was added to fix. Twenty-six unit tests passed while
// the fix was completely inert, because they all tested the routing function
// and none tested what was fed to it. This is that test (#4132).
func TestSignerOf_ReadsSignatureMessages(t *testing.T) {
	signer := url.MustParse("acc://alice.acme/book/1")
	txid := url.MustParse("acc://alice.acme/tokens").WithTxID([32]byte{1})

	env := &messaging.Envelope{
		Messages: []messaging.Message{
			&messaging.SignatureMessage{
				Signature: &protocol.ED25519Signature{Signer: signer},
				TxID:      txid,
			},
		},
	}

	assert.Equal(t, signer.String(), signerOf(env),
		"the signer must come from the signature MESSAGE, not the legacy field")
}

// The legacy field still works, for anything that populates it.
func TestSignerOf_FallsBackToLegacySignatures(t *testing.T) {
	signer := url.MustParse("acc://bob.acme/book/1")
	env := &messaging.Envelope{
		Signatures: []protocol.Signature{&protocol.ED25519Signature{Signer: signer}},
	}
	assert.Equal(t, signer.String(), signerOf(env))
}

// The message form wins when both are present, since that is what real
// submissions carry.
func TestSignerOf_PrefersMessagesOverLegacy(t *testing.T) {
	fromMsg := url.MustParse("acc://from-message.acme/book/1")
	fromLegacy := url.MustParse("acc://from-legacy.acme/book/1")
	env := &messaging.Envelope{
		Messages: []messaging.Message{
			&messaging.SignatureMessage{
				Signature: &protocol.ED25519Signature{Signer: fromMsg},
				TxID:      url.MustParse("acc://x.acme").WithTxID([32]byte{2}),
			},
		},
		Signatures: []protocol.Signature{&protocol.ED25519Signature{Signer: fromLegacy}},
	}
	assert.Equal(t, fromMsg.String(), signerOf(env))
}

// An envelope with nothing to key on degrades to round-robin rather than
// panicking. Empty is a valid answer; a crash is not.
func TestSignerOf_EmptyEnvelopeIsSafe(t *testing.T) {
	assert.Equal(t, "", signerOf(&messaging.Envelope{}))
	assert.Equal(t, "", signerOf(&messaging.Envelope{
		Messages: []messaging.Message{
			&messaging.TransactionMessage{},
		},
	}))
}

// A signature message with a nil signature must not panic — envelopes arrive
// from the network and cannot be assumed well-formed.
func TestSignerOf_NilSignatureIsSafe(t *testing.T) {
	env := &messaging.Envelope{
		Messages: []messaging.Message{&messaging.SignatureMessage{Signature: nil}},
	}
	assert.NotPanics(t, func() { _ = signerOf(env) })
	assert.Equal(t, "", signerOf(env))
}

// Two transactions from the same signer must produce the same key — the whole
// point — and different signers different keys.
func TestSignerOf_StableAcrossTransactionsFromOneSigner(t *testing.T) {
	mk := func(s string, tx byte) *messaging.Envelope {
		return &messaging.Envelope{
			Messages: []messaging.Message{
				&messaging.SignatureMessage{
					Signature: &protocol.ED25519Signature{Signer: url.MustParse(s)},
					TxID:      url.MustParse("acc://x.acme").WithTxID([32]byte{tx}),
				},
			},
		}
	}
	a1 := signerOf(mk("acc://treasury.acme/ACME", 1))
	a2 := signerOf(mk("acc://treasury.acme/ACME", 2))
	b := signerOf(mk("acc://other.acme/ACME", 3))

	require.NotEmpty(t, a1)
	assert.Equal(t, a1, a2, "same signer, different transactions -> same key")
	assert.NotEqual(t, a1, b, "different signers -> different keys")
}
