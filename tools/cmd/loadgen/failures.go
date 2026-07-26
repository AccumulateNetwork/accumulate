// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The fail:* actions deliberately submit transactions that are valid enough to
// EXECUTE and be charged a fee, then fail on business logic — forcing the
// fee-refund path (FeeRefund). They exercise the refund machinery AND the
// accounting reconciler: a refund lands asynchronously, so the local credit
// mirror (debited at submit) drifts until reconcile re-syncs it. The clean
// refund producers are overspend/overburn/send-to-void (signature valid,
// execution fails); the "target does not exist" cases execute under a real
// identity's authority over a path that is missing.
//
// All are marked expectFail so the delivery tracker does not follow them —
// they are supposed not to deliver normally, and following them would count
// them toward -max-stranded.

// sendTokensToVoid sends ACME to a token account under an ADI that does not
// exist. The source executes and produces the synthetic deposit (charged); the
// deposit fails at the destination (no such account) and a refund routes back.
// Cross-partition refund path.
var sendTokensToVoid = action{
	name: "fail:send-to-void", weight: 2, expectFail: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		src := e.u.randSourceLite()
		if src == nil {
			return nil, errors.NotReady.With("no funded source")
		}
		dest := adiName("void").JoinPath("tokens")
		return e.sign(ctx, src.id, func() txBuilder {
			return e.build(src).
				SendTokens(sendAmount, protocol.AcmePrecisionPower).To(dest).
				SignWith(src.id).Version(1).Timestamp(e.nonce.next()).PrivateKey(src.key)
		})
	},
}

// overSpendTokens sends far more ACME than the source holds. The transfer
// executes, fails on insufficient balance, and refunds.
var overSpendTokens = action{
	name: "fail:overspend", weight: 2, expectFail: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		src := e.u.randSourceLite()
		if src == nil {
			return nil, errors.NotReady.With("no funded source")
		}
		to := e.u.randLite()
		if to == nil || to == src {
			return nil, errors.NotReady.With("no recipient")
		}
		return e.sign(ctx, src.id, func() txBuilder {
			return e.build(src).
				SendTokens(1e9, protocol.AcmePrecisionPower).To(to.acct). // >> any balance
				SignWith(src.id).Version(1).Timestamp(e.nonce.next()).PrivateKey(src.key)
		})
	},
}

// NOTE: there is deliberately no fail:overburn-credits action. Burning more
// credits than the page holds does NOT reach execution — BurnCredits validates
// the balance at submit, so the envelope is rejected outright with
// insufficientBalance. Nothing is charged, so there is no fee to refund and the
// refund path is never exercised: the action produced only rejection noise in
// the loadgen stats, which obscures the real rejections a soak needs to
// surface. The refund path is covered by fail:overspend, fail:overburn-tokens,
// fail:send-to-void, and the two void-target cases, all of which are valid
// enough to execute and be charged before failing on business logic.

// overBurnTokens burns far more ACME than an ADI token account holds. Executes,
// fails on insufficient balance, refunds.
var overBurnTokens = action{
	name: "fail:overburn-tokens", weight: 1, needsIdentity: true, expectFail: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		a := e.u.randIdentity()
		if a == nil || a.signer() == nil || len(a.tokens) == 0 {
			return nil, errors.NotReady.With("no funded token account")
		}
		s := a.signer()
		return e.sign(ctx, s.url, func() txBuilder {
			return e.build(a.tokens[0]).
				BurnTokens(1e9, protocol.AcmePrecisionPower). // >> any balance
				SignWith(s.url).Version(s.version).Timestamp(e.nonce.next()).PrivateKey(a.key())
		})
	},
}

// createSubAdiOnVoid creates a sub-ADI under an intermediate that does not
// exist, signed by a real identity whose authority covers the whole namespace.
// The signature is valid, so it executes and is charged; execution fails because
// the parent path is missing, and refunds.
var createSubAdiOnVoid = action{
	name: "fail:sub-adi-on-void", weight: 1, needsIdentity: true, expectFail: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		a := e.u.randIdentity()
		if a == nil || a.signer() == nil {
			return nil, errors.NotReady.With("no signer")
		}
		s := a.signer()
		ghost := a.url.JoinPath(fmt.Sprintf("ghost%d", e.u.intn(1<<20))) // missing intermediate
		child := ghost.JoinPath("child")
		k := newKey()
		return e.sign(ctx, s.url, func() txBuilder {
			return e.build(ghost). // principal = the missing parent
				CreateIdentity(child).WithKey(k, protocol.SignatureTypeED25519).WithKeyBook(child.JoinPath("book")).
				SignWith(s.url).Version(s.version).Timestamp(e.nonce.next()).PrivateKey(a.key())
		})
	},
}

// writeDataToVoid writes to an ADI data account that does not exist, under a
// real identity's authority. ADI data accounts are not auto-created on write, so
// it executes and fails on the missing account, and refunds.
var writeDataToVoid = action{
	name: "fail:data-to-void", weight: 1, needsIdentity: true, expectFail: true,
	run: func(ctx context.Context, e *env) ([]*url.TxID, error) {
		a := e.u.randIdentity()
		if a == nil || a.signer() == nil {
			return nil, errors.NotReady.With("no signer")
		}
		s := a.signer()
		ghost := a.url.JoinPath(fmt.Sprintf("ghostdata%d", e.u.intn(1<<20))) // missing data account
		entry := &protocol.DoubleHashDataEntry{Data: [][]byte{[]byte("loadgen-void")}}
		return e.sign(ctx, s.url, func() txBuilder {
			return e.build(ghost).
				WriteData().Entry(entry).
				SignWith(s.url).Version(s.version).Timestamp(e.nonce.next()).PrivateKey(a.key())
		})
	},
}
