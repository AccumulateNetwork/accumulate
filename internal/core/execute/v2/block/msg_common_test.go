// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/internal"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// isWithin used to ignore its arguments: two of its three switch arms never
// looked at `typ`, so isWithin(anything) returned true for every message
// inside a synthetic wrapper (#4168). The argument list was decoration.
//
// These pin that it now answers the question it is asked.

// msgOfType returns a real message of the given type. Real ones rather than a
// stub, so the chain is built out of what actually flows through the executor.
func msgOfType(t *testing.T, ty messaging.MessageType) messaging.Message {
	t.Helper()
	switch ty {
	case messaging.MessageTypeSynthetic:
		return new(messaging.SyntheticMessage)
	case messaging.MessageTypeBadSynthetic:
		return new(messaging.BadSyntheticMessage)
	case messaging.MessageTypeSequenced:
		return new(messaging.SequencedMessage)
	case messaging.MessageTypeBlockAnchor:
		return new(messaging.BlockAnchor)
	case messaging.MessageTypeTransaction:
		return new(messaging.TransactionMessage)
	case messaging.MessageTypeCreditPayment:
		return new(messaging.CreditPayment)
	case internal.MessageTypeMessageIsReady:
		return new(internal.MessageIsReady)
	default:
		t.Fatalf("no fixture for %v", ty)
		return nil
	}
}

// chainOf builds a context chain, outermost first, and returns a leaf below
// it. isWithin never inspects the context it is called on, only its ancestors.
func chainOf(t *testing.T, types ...messaging.MessageType) *MessageContext {
	t.Helper()
	// Executor reaches MessageContext through the embedded bundle and Block,
	// so the whole chain shares one bundle.
	x := streamTestExec(t)
	d := &bundle{Block: &Block{positions: new(positionCache), Executor: x}}

	var ctx *MessageContext
	for _, ty := range types {
		c := &MessageContext{bundle: d, message: msgOfType(t, ty), parent: ctx}
		ctx = c
	}
	return &MessageContext{bundle: d, message: msgOfType(t, messaging.MessageTypeTransaction), parent: ctx}
}

func TestIsWithin_AnswersTheQuestionItIsAsked(t *testing.T) {
	// A synthetic wrapper in the chain, asked about something else entirely.
	// This is the bug: it used to say yes.
	ctx := chainOf(t, messaging.MessageTypeSynthetic, messaging.MessageTypeSequenced)
	assert.False(t, ctx.isWithin(internal.MessageTypeMessageIsReady),
		"a synthetic wrapper is not a MessageIsReady, and asking about one must not match the other")
	assert.False(t, ctx.isWithin(messaging.MessageTypeBlockAnchor),
		"nor a BlockAnchor")
	assert.True(t, ctx.isWithin(messaging.MessageTypeSynthetic),
		"but asking about synthetic still finds it")
	assert.True(t, ctx.isWithin(messaging.MessageTypeSequenced),
		"and a type actually in the chain is found")
}

func TestIsWithin_FindsWhatIsThere(t *testing.T) {
	ctx := chainOf(t, internal.MessageTypeMessageIsReady, messaging.MessageTypeSynthetic, messaging.MessageTypeSequenced)
	assert.True(t, ctx.isWithin(internal.MessageTypeMessageIsReady))
	assert.True(t, ctx.isWithin(messaging.MessageTypeSynthetic))
	assert.True(t, ctx.isWithin(messaging.MessageTypeSequenced, messaging.MessageTypeBlockAnchor),
		"any of the listed types matching is enough")
	assert.False(t, ctx.isWithin(messaging.MessageTypeCreditPayment))
}

// Asking about synthetic matches EITHER wrapper, because which one is used
// depends on the executor version. That special case is why the arms existed;
// it just was not conditioned on being asked.
func TestIsWithin_SyntheticMatchesEitherWrapper(t *testing.T) {
	bad := chainOf(t, messaging.MessageTypeBadSynthetic, messaging.MessageTypeSequenced)
	assert.True(t, bad.isWithin(messaging.MessageTypeSynthetic),
		"the old wrapper answers to synthetic")

	good := chainOf(t, messaging.MessageTypeSynthetic, messaging.MessageTypeSequenced)
	assert.True(t, good.isWithin(messaging.MessageTypeSynthetic),
		"and so does the new one")
}

func TestIsWithin_NothingInTheChain(t *testing.T) {
	ctx := chainOf(t, messaging.MessageTypeTransaction)
	assert.False(t, ctx.isWithin(messaging.MessageTypeSynthetic))
	assert.False(t, ctx.isWithin(internal.MessageTypeMessageIsReady))
	assert.False(t, ctx.isWithin())
}
