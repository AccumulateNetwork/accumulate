// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// lineCapture records the lines a block writes under one message.
type lineCapture struct {
	msgs  []string
	lines []map[string]any
}

func (c *lineCapture) Debug(string, ...interface{})       {}
func (c *lineCapture) Error(string, ...interface{})       {}
func (c *lineCapture) With(...interface{}) logging.Logger { return c }
func (c *lineCapture) Info(msg string, kv ...interface{}) {
	f := map[string]any{}
	for i := 0; i+1 < len(kv); i += 2 {
		if k, ok := kv[i].(string); ok {
			f[k] = kv[i+1]
		}
	}
	c.msgs = append(c.msgs, msg)
	c.lines = append(c.lines, f)
}

func (c *lineCapture) named(msg string) []map[string]any {
	var out []map[string]any
	for i, m := range c.msgs {
		if m == msg {
			out = append(out, c.lines[i])
		}
	}
	return out
}

// #4465: after the restart BVN2 minted 15 to 80 times as many synthetics for
// BVN1 as before, all genuinely new, and no line said what they were or what
// caused them. The block's produced messages are counted by destination and
// kind and their causes by kind, and written as one line no more often than
// ProducedKindsEvery; a cause that produces again, in a later execution, what
// it produced before is named with how many times.
func TestProducedMessagesAreLoggedByKindAndCause(t *testing.T) {
	s := newStagingSim(t, 2)
	c := new(lineCapture)
	s.x.logger.Set(c)
	t0 := time.Date(2026, 9, 29, 5, 0, 0, 0, time.UTC)

	// The cause: a sendTokens transaction on this partition
	cause := new(protocol.Transaction)
	cause.Header.Principal = protocol.AccountUrl("alice", "tokens")
	cause.Body = &protocol.SendTokens{To: []*protocol.TokenRecipient{{Url: protocol.AccountUrl("bob", "tokens")}}}
	causeMsg := &messaging.TransactionMessage{Transaction: cause}

	// One execution of the cause produces one deposit, through the
	// production path every producer takes
	execute := func(amount uint64) []*ProducedMessage {
		t.Helper()
		ctx := &MessageContext{bundle: &bundle{Block: s.b, batch: s.batch}, message: causeMsg}
		txn := new(protocol.Transaction)
		txn.Header.Principal = protocol.AccountUrl("bob", "tokens")
		body := new(protocol.SyntheticDepositTokens)
		body.Token = protocol.AcmeUrl()
		body.Amount.SetUint64(amount)
		txn.Body = body
		require.NoError(t, ctx.didProduce(s.batch, protocol.AccountUrl("bob", "tokens"), &messaging.TransactionMessage{Transaction: txn}))
		return ctx.produced
	}
	bvn1 := []*url.URL{protocol.PartitionUrl("BVN1")}

	// First execution: not a re-emission
	s.b.Index, s.b.Time = 10, t0
	p := execute(1)
	require.Len(t, p, 1)
	require.Zero(t, p[0].causeBefore, "the cause had produced nothing before")
	s.b.noteProducedKinds(p, bvn1)
	lines := c.named("Produced messages")
	require.Len(t, lines, 1)
	require.Equal(t, "10-10", lines[0]["blocks"])
	require.Equal(t, 1, lines[0]["count"])
	require.Equal(t, "BVN1 transaction/syntheticDepositTokens=1", lines[0]["produced"])
	require.Equal(t, "transaction/sendTokens=1", lines[0]["causes"])
	require.Empty(t, c.named("Cause re-emitted"))

	// The same cause executes again within the window, twice: counted,
	// not yet logged
	s.b.Index, s.b.Time = 11, t0.Add(time.Second)
	p = execute(2)
	require.Equal(t, 1, p[0].causeBefore, "the cause had produced one message before")
	s.b.noteProducedKinds(p, bvn1)
	s.b.Index, s.b.Time = 12, t0.Add(2*time.Second)
	s.b.noteProducedKinds(execute(3), bvn1)
	require.Len(t, c.named("Produced messages"), 1, "rate-limited")

	// A block with nothing produced past the window writes what was counted
	s.b.Index, s.b.Time = 13, t0.Add(ProducedKindsEvery)
	s.b.noteProducedKinds(nil, nil)
	lines = c.named("Produced messages")
	require.Len(t, lines, 2)
	require.Equal(t, "11-12", lines[1]["blocks"])
	require.Equal(t, 2, lines[1]["count"])
	re := c.named("Cause re-emitted")
	require.Len(t, re, 1)
	h := causeMsg.Hash()
	require.True(t, strings.Contains(re[0]["causes"].(string), "transaction/sendTokens before=2 times=2"), re[0]["causes"])
	require.True(t, strings.HasPrefix(re[0]["causes"].(string), hex.EncodeToString(h[:])), re[0]["causes"])

	// Nothing counted: nothing written
	s.b.Index, s.b.Time = 14, t0.Add(time.Hour)
	s.b.noteProducedKinds(nil, nil)
	require.Len(t, c.named("Produced messages"), 2)
}
