// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package harness

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A Condition is a function that is used by Harness.StepUntil to wait until
// some condition is met.
type Condition interface {
	Satisfied(*Harness) bool
	Format(prefix, suffix string) string
}

type True func(*Harness) bool

func (f True) Satisfied(h *Harness) bool { return f(h) }

func (f True) Format(prefix, suffix string) string { return prefix + "(unknown predicate function)" }

type False func(*Harness) bool

func (f False) Satisfied(h *Harness) bool { return !f(h) }

func (f False) Format(prefix, suffix string) string { return prefix + "(unknown predicate function)" }

func MajorBlock(v uint64) majorHeightOnPart {
	return majorHeightOnPart{v, protocol.Directory}
}

type majorHeightOnPart struct {
	height    uint64
	partition string
}

func (v majorHeightOnPart) OnPartition(s string) majorHeightOnPart {
	return majorHeightOnPart{v.height, s}
}

func (v majorHeightOnPart) Satisfied(h *Harness) bool {
	ns := h.NetworkStatus(api.NetworkStatusOptions{Partition: v.partition})
	return ns.MajorBlockHeight >= v.height
}

func (v majorHeightOnPart) Format(prefix, suffix string) string {
	return fmt.Sprintf("%s%s reached major block %d%s", prefix, v.partition, v.height, suffix)
}

func DnHeight(v uint64) dnHeightOnPart {
	return dnHeightOnPart{v, protocol.Directory}
}

type dnHeightOnPart struct {
	height    uint64
	partition string
}

func (v dnHeightOnPart) OnPartition(s string) dnHeightOnPart {
	return dnHeightOnPart{v.height, s}
}

func (v dnHeightOnPart) Satisfied(h *Harness) bool {
	ns := h.NetworkStatus(api.NetworkStatusOptions{Partition: v.partition})
	return ns.DirectoryHeight >= v.height
}

func (v dnHeightOnPart) Format(prefix, suffix string) string {
	return fmt.Sprintf("%sDN block %d is anchored on %s%s", prefix, v.height, v.partition, suffix)
}

func BlockTime(t time.Time) blockTimeOnPart {
	return blockTimeOnPart{t, protocol.Directory}
}

type blockTimeOnPart struct {
	time      time.Time
	partition string
}

func (v blockTimeOnPart) OnPartition(s string) blockTimeOnPart {
	return blockTimeOnPart{v.time, s}
}

func (v blockTimeOnPart) Satisfied(h *Harness) bool {
	cs := h.ConsensusStatus(api.ConsensusStatusOptions{Partition: v.partition})
	return !cs.LastBlock.Time.Before(v.time)
}

func (v blockTimeOnPart) Format(prefix, suffix string) string {
	return fmt.Sprintf("%s%s reached block time %v%s", prefix, v.partition, v.time, suffix)
}

func VersionIs(v protocol.ExecutorVersion) executorVersion {
	if !v.V2VandenbergEnabled() {
		panic("this feature is not available for versions prior to v2-vandenberg")
	}
	return executorVersion{version: v}
}

type executorVersion struct {
	version protocol.ExecutorVersion
}

func (v executorVersion) Satisfied(h *Harness) bool {
	ns := h.NetworkStatus(api.NetworkStatusOptions{Partition: protocol.Directory})
	if len(ns.BvnExecutorVersions) == 0 {
		return false
	}
	if ns.ExecutorVersion < v.version {
		return false
	}
	for _, b := range ns.BvnExecutorVersions {
		if b.Version < v.version {
			return false
		}
	}
	return true
}

func (v executorVersion) Format(prefix, suffix string) string {
	return fmt.Sprintf("%sexecutor version is %v%s", prefix, v.version, suffix)
}

// Msg defines a condition on a message.
func Msg(id *url.TxID) msgCond {
	return msgCond{id: id, message: []string{"message " + id.ShortString()}}
}

// Txn defines a condition on a transaction.
func Txn(id *url.TxID) txnCond {
	return txnCond{msgCond{id: id, message: []string{"transaction " + id.ShortString()}}}
}

// Sig defines a condition on a signature.
func Sig(id *url.TxID) sigCond {
	return sigCond{msgCond{id: id, message: []string{"signature " + id.ShortString()}}}
}

// txnCond provides methods to define conditions on a transaction.
type txnCond struct{ msgCond }

// Produced defines a condition on the synthetic transaction(s) produced by a
// transaction.
func (c txnCond) Produced() msgCond { return c.with("produced", deliveredThen, produced) }

// Refund defines a condition on the refund transaction(s) produced by the
// synthetic transaction(s) produced by a transaction.
func (c txnCond) Refund() msgCond {
	return c.with("refund", deliveredThen, produced, deliveredThen, produced)
}

// sigCond provides methods to define conditions on a signature.
type sigCond struct{ msgCond }

// SingleCompletes waits for the signature, credit payment, signature requests,
// and authority signatures to complete. SingleCompletes is inappropriate for
// signatures from multi-sig or lite identity signers.
func (c sigCond) SingleCompletes() Condition {
	conditions := []Condition{
		c.Succeeds(),
		c.CreditPayment().Succeeds(),
		c.SignatureRequest().Completes(),
		c.AuthoritySignature().Completes(),
	}
	return c.make("completes", func(h *Harness, _ *condition, _ *msgResult) bool {
		ok := true
		for _, c := range conditions {
			if !c.Satisfied(h) {
				ok = false
			}
		}
		return ok
	})
}

// LiteCompletes waits for the signature, credit payment, and authority
// signatures to complete. LiteCompletes is inappropriate for signatures from
// multi-sig signers.
func (c sigCond) LiteCompletes() Condition {
	conditions := []Condition{
		c.Succeeds(),
		c.CreditPayment().Succeeds(),
		c.AuthoritySignature().Completes(),
	}
	return c.make("completes", func(h *Harness, _ *condition, _ *msgResult) bool {
		ok := true
		for _, c := range conditions {
			if !c.Satisfied(h) {
				ok = false
			}
		}
		return ok
	})
}

// CreditPayment defines a condition on the credit payment produced by a
// signature.
func (c sigCond) CreditPayment() msgCond {
	return c.with("credit payment", deliveredThen, producedFiltered(func(r *msgResult) bool {
		_, ok := messaging.UnwrapAs[*messaging.CreditPayment](r.Message)
		return ok
	}))
}

// SignatureRequest defines a condition on signature requests produced by a
// signature.
func (c sigCond) SignatureRequest() sigCond {
	return sigCond{c.with("signature request", deliveredThen, producedFiltered(func(r *msgResult) bool {
		_, ok := messaging.UnwrapAs[*messaging.SignatureRequest](r.Message)
		return ok
	}))}
}

// SignatureRequestTo defines a condition on signature requests produced by a
// signature.
func (c sigCond) SignatureRequestTo(auth *url.URL, path ...string) sigCond {
	auth = auth.JoinPath(path...)
	return sigCond{c.with("signature request", deliveredThen, producedFiltered(func(r *msgResult) bool {
		msg, ok := messaging.UnwrapAs[*messaging.SignatureRequest](r.Message)
		return ok && msg.Authority.Equal(auth)
	}))}
}

// AuthoritySignature defines a condition on the authority signature produced by
// a signature.
func (c sigCond) AuthoritySignature() sigCond {
	return sigCond{c.with("authority signature", deliveredThen, producedFiltered(func(r *msgResult) bool {
		msg, ok := messaging.UnwrapAs[*messaging.SignatureMessage](r.Message)
		return ok && msg.Signature.Type() == protocol.SignatureTypeAuthority
	}))}
}

// Produced defines a condition on messages produced by a signature request or
// authority signature.
func (c sigCond) Produced() sigCond { return sigCond{c.with("produced", deliveredThen, produced)} }

// msgCond provides methods to define conditions on a message.
type msgCond struct {
	id        *url.TxID
	modifiers []predicateModifier
	message   []string
	capture   **protocol.TransactionStatus
}

func (c msgCond) with(message string, mod ...predicateModifier) msgCond {
	// Reverse the modifiers since nesting is the inverse of the natural order
	for i, n := 0, len(mod); i < n/2; i++ {
		mod[i], mod[n-i-1] = mod[n-i-1], mod[i]
	}

	parts := make([]string, len(c.message)+1)
	n := copy(parts, c.message)
	parts[n] = message

	return msgCond{
		id:        c.id,
		modifiers: append(mod, c.modifiers...),
		message:   parts,
	}
}

func (c msgCond) make(message string, predicate statusPredicate) *condition {
	parts := make([]string, len(c.message)+1)
	n := copy(parts, c.message)
	parts[n] = message
	for _, mod := range c.modifiers {
		predicate = mod(predicate)
	}
	return &condition{
		predicate: waitFor(c.id)(predicate),
		message:   parts,
		capture:   c.capture,
	}
}

// Capture captures the status of the message in the given pointer. If the
// condition involves multiple messages, the captured status will be the status
// of the last message.
func (c msgCond) Capture(ptr **protocol.TransactionStatus) msgCond {
	c.capture = ptr
	return c
}

// Received waits until the messages has been received.
func (c msgCond) Received() Condition { return c.make("is received", received) }

// IsDelivered waits until the messages has been delivered (executed, whether
// success or failure).
func (c msgCond) IsDelivered() Condition { return c.make("is delivered", isDelivered) }

// IsPending waits until the messages is pending (received but not executed).
// IsPending will fail if the messages has been recorded with any status other
// than pending.
func (c msgCond) IsPending() Condition { return c.make("is pending", isPending) }

// Succeeds waits until the messages has been delivered and succeeds if the
// messages succeeded (and fails otherwise).
func (c msgCond) Succeeds() Condition { return c.make("succeeds", deliveredThen(succeeds)) }

// Completes waits until the messages has been delivered, verifies the
// messages succeeded, and recursively waits on any produced messages.
func (c msgCond) Completes() Condition {
	return c.make("completes", producedRecursive(succeeds))
}

type msgResult struct {
	Type     string
	Message  messaging.Message
	Status   *protocol.TransactionStatus
	Produced []*url.TxID
}

type statusPredicate func(h *Harness, c *condition, r *msgResult) bool

type predicateModifier func(statusPredicate) statusPredicate

type condition struct {
	children   []*condition
	predicate  statusPredicate
	lastResult *msgResult
	capture    **protocol.TransactionStatus
	message    []string
	prodMsg    *string
}

func (c *condition) Format(prefix, suffix string) string {
	if c.children == nil {
		return prefix + strings.Join(c.message, " ") + suffix
	}
	var s string
	for _, c := range c.children {
		s += c.Format(prefix, suffix)
	}
	return s
}

func (c *condition) messageReplaceEnd(s string) string {
	if c.children == nil {
		t := strings.Join(c.message[:len(c.message)-1], " ")
		return t + " " + s
	}
	var t string
	for _, c := range c.children {
		t += c.messageReplaceEnd(s)
	}
	return t
}

func (c *condition) Satisfied(h *Harness) bool {
	h.TB.Helper()
	if c.children == nil {
		return c.predicate(h, c, c.lastResult)
	}
	ok := true
	for _, c := range c.children {
		if !c.Satisfied(h) {
			ok = false
		}
	}
	return ok
}

func (c *condition) replacePredicate(h *Harness, new statusPredicate) bool {
	h.TB.Helper()
	c.predicate = new
	return c.Satisfied(h)
}

func (c *condition) replaceWithCondition(h *Harness, d *condition) bool {
	h.TB.Helper()
	c.message = d.message
	c.prodMsg = d.prodMsg
	return c.replacePredicate(h, d.predicate)
}

func (c *condition) replaceWithMultiple(h *Harness, d []*condition) bool {
	h.TB.Helper()
	c.children = d
	c.predicate = nil
	return c.Satisfied(h)
}

func getMessageResult(h *Harness, id *url.TxID) (*msgResult, bool) {
	// Query the transaction
	h.TB.Helper()
	qr, err := h.Query().Query(context.Background(), id.AsUrl(), new(api.DefaultQuery))
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		return nil, false

	default:
		// Unknown error
		require.NoError(h.TB, err)
		panic("not reached")
	}

	msg, ok := qr.(*api.MessageRecord[messaging.Message])
	if !ok {
		h.TB.Fatalf("Unsupported record type %v", qr.RecordType())
		panic("not reached")
	}

	// Convert the result
	var res msgResult
	res.Status = &protocol.TransactionStatus{
		TxID:     id,
		Code:     msg.Status,
		Error:    msg.Error,
		Result:   msg.Result,
		Received: msg.Received,
	}
	if msg.Produced != nil {
		for _, v := range msg.Produced.Records {
			res.Produced = append(res.Produced, v.Value)
		}
	}
	res.Message = msg.Message

	switch msg := msg.Message.(type) {
	case *messaging.SignatureMessage:
		res.Type = msg.Signature.Type().String() + " signature"
	case *messaging.TransactionMessage:
		res.Type = msg.Transaction.Body.Type().String() + " transaction"
	default:
		res.Type = msg.Type().String()
	}

	return &res, true
}

func waitFor(id *url.TxID) predicateModifier {
	return func(predicate statusPredicate) statusPredicate {
		return func(h *Harness, c *condition, _ *msgResult) bool {
			// Wait for the message
			h.TB.Helper()
			r, ok := getMessageResult(h, id)
			if !ok {
				return false
			}

			c.lastResult = r
			if c.capture != nil {
				*c.capture = r.Status
			}

			if c.prodMsg != nil {
				*c.prodMsg = "produced " + r.Type
				c.prodMsg = nil
			}

			// Evaluate the predicate (only replace if the status is final)
			if r.Status.Delivered() {
				return c.replacePredicate(h, predicate)
			}
			return predicate(h, c, r)
		}
	}
}

func deliveredThen(predicate statusPredicate) statusPredicate {
	return func(h *Harness, c *condition, r *msgResult) bool {
		h.TB.Helper()

		// If the transaction is pending, wait
		if !r.Status.Delivered() {
			return false
		}

		return c.replacePredicate(h, predicate)
	}
}

func produced(predicate statusPredicate) statusPredicate {
	return func(h *Harness, c *condition, r *msgResult) bool {
		h.TB.Helper()

		// Expect produced transactions
		if len(r.Produced) == 0 {
			h.TB.Fatalf("%v (%v) did not produce anything", r.Status.TxID, r.Type)
		}

		nProd := len(c.message) - 2
		if nProd < 0 || c.message[nProd] != "produced" {
			nProd = -1
		}

		conditions := make([]*condition, len(r.Produced))
		for i, id := range r.Produced {
			c := &condition{
				predicate: waitFor(id)(predicate),
				message:   c.message,
				capture:   c.capture,
			}
			conditions[i] = c
			if nProd < 0 {
				continue
			}

			m := make([]string, len(c.message))
			copy(m, c.message)
			if nProd > 0 {
				m[nProd-1] = r.Type
			}
			c.prodMsg = &m[nProd]
			c.message = m
		}

		if len(conditions) == 1 {
			return c.replaceWithCondition(h, conditions[0])
		}
		return c.replaceWithMultiple(h, conditions)
	}
}

func always(*Harness, *condition, *msgResult) bool { return true }

func producedRecursive(predicate statusPredicate) statusPredicate {
	var recursive statusPredicate
	var isSig bool
	recursive = func(h *Harness, c *condition, r *msgResult) bool {
		h.TB.Helper()

		// If a signature produces a transaction, ignore it. Sig(id).Completes()
		// should *not* wait for the *transaction* to complete.
		if r.Message.Type() == messaging.MessageTypeSignature {
			isSig = true
		}
		if isSig && r.Message.Type() == messaging.MessageTypeTransaction {
			return c.replacePredicate(h, always)
		}

		// If the transaction is pending, wait
		if !r.Status.Delivered() {
			return false
		}

		conditions := make([]*condition, len(r.Produced)+1)
		conditions[0] = &condition{
			predicate:  predicate,
			message:    c.message,
			capture:    c.capture,
			lastResult: r,
		}

		for i, id := range r.Produced {
			m := make([]string, len(c.message)+1)
			n := copy(m, c.message[:len(c.message)-1])
			m[n] = "produced"
			m[n+1] = c.message[n]
			conditions[i+1] = &condition{
				predicate: waitFor(id)(recursive),
				message:   m,
				capture:   c.capture,
				prodMsg:   &m[n],
			}
		}

		if len(conditions) == 1 {
			return c.replaceWithCondition(h, conditions[0])
		}
		return c.replaceWithMultiple(h, conditions)
	}
	return recursive
}

func producedFiltered(filter func(*msgResult) bool) predicateModifier {
	return func(predicate statusPredicate) statusPredicate {
		return func(h *Harness, c *condition, r *msgResult) bool {
			h.TB.Helper()

			// Expect produced transactions
			if len(r.Produced) == 0 {
				h.TB.Fatalf("%v (%v) did not produce anything", r.Status.TxID, r.Type)
			}

			// For each produced
			for _, id := range r.Produced {
				// Wait for it
				r, ok := getMessageResult(h, id)
				if !ok {
					continue
				}

				// Is it the one we want?
				if !filter(r) {
					continue
				}

				if n := len(c.message) - 2; n >= 0 && c.message[n] == "produced" {
					c.prodMsg = &c.message[n]
				}

				// Found it, delegate to the predicate
				return c.replacePredicate(h, waitFor(id)(predicate))
			}

			// The message we want hasn't appeared yet
			return false
		}
	}
}

func received(h *Harness, c *condition, r *msgResult) bool {
	return c.replacePredicate(h, func(h *Harness, c *condition, r *msgResult) bool {
		h.TB.Helper()
		return r.Status.Code != 0
	})
}

func isDelivered(h *Harness, c *condition, r *msgResult) bool {
	return c.replacePredicate(h, func(h *Harness, c *condition, r *msgResult) bool {
		h.TB.Helper()
		return r.Status.Delivered()
	})
}

func isPending(h *Harness, c *condition, r *msgResult) bool {
	h.TB.Helper()

	// Check if the transaction is recorded as pending
	if r.Status.Code != 0 {
		// Must be pending
		if r.Status.Code == errors.Pending {
			return true
		}
		if r.Status.Error == nil {
			h.TB.Logf("transaction is %v\n", r.Status.Code)
		} else {
			h.TB.Logf("%+v\n", r.Status.AsError())
		}
		h.TB.Fatal(c.messageReplaceEnd("is not pending 🗴\n"))
	}

	// Check if the account lists the transaction as pending
	qr, err := h.Query().QueryPendingIds(context.Background(), r.Status.TxID.Account(), nil)
	switch {
	case err == nil:
		for _, qr := range qr.Records {
			if qr.Value.Hash() == r.Status.TxID.Hash() {
				return true
			}
		}

	case !errors.Is(err, errors.NotFound):
		// Unknown error
		require.NoError(h.TB, err)
		panic("not reached")
	}

	return false
}

func succeeds(h *Harness, c *condition, r *msgResult) bool {
	h.TB.Helper()

	// Must be success
	if r.Status.Failed() {
		h.TB.Logf("%+v\n", r.Status.AsError())
		h.TB.Fatal(c.messageReplaceEnd("did not succeed 🗴\n"))
	}
	if h.VerboseConditions {
		fmt.Print(c.Format("", " ✔\n"))
	}
	return c.replacePredicate(h, always)
}
