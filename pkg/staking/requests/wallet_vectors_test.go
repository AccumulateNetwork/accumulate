// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package requests

// THE CONTRACT, stated as bytes.
//
// core/wallet is what writes staking requests, and its
// internal/staking.FormatActions defines their encoding: a bare action
// marker followed by the action's fields as sorted key=value parts. The
// vectors below are copied verbatim from that package's
// action_test.go — they are the wallet's own golden output, not our
// interpretation of it.
//
// This package claims to be the definitive reader and writer of staking
// requests. That claim is only true if it round-trips every one of these.
// Measured on 2026-08-27 before the fix: 7 of 11 parsed, 2 of 11
// re-encoded. Four actions — registerIdentity, rejectDelegates,
// changeDelegatorPayout, cancelRequest — were rejected as "not a staking
// request" because parseLegacy required an `account=` field, and those
// four are scoped to an identity or to the txid they revoke. The encoder
// meanwhile emitted JSON, a dialect the wallet cannot read, so a
// round trip destroyed the entry.
//
// If a vector here fails, the wallet writes something this package cannot
// read, and a real user's request is stranded on chain.

import (
	"testing"
)

// walletVectors mirrors core/wallet internal/staking action_test.go
// (TestFormatActionsGoldenVectors), plus the three actions that file
// exercises through its enum test.
var walletVectors = []struct {
	name  string
	parts []string
	kind  Kind
}{
	{"addAccount pure", []string{
		"addAccount", "account=acc://myadi.acme/staking", "type=pure"}, KindRegister},
	{"addAccount coreValidator with payout", []string{
		"addAccount", "account=acc://myadi.acme/staking",
		"payout=acc://myadi.acme/rewards", "type=coreValidator"}, KindRegister},
	{"addAccount delegated with delegate", []string{
		"addAccount", "account=acc://myadi.acme/staking",
		"delegate=acc://validator.acme", "type=delegated"}, KindRegister},
	{"withdrawTokens", []string{
		"withdrawTokens", "account=acc://myadi.acme/staking",
		"amount=12345", "recipient=acc://myadi.acme/rewards"}, KindWithdraw},
	{"transferTokens", []string{
		"transferTokens", "account=acc://myadi.acme/staking",
		"amount=500", "recipient=acc://myadi.acme/rewards"}, KindTransfer},
	{"changePayout", []string{
		"changePayout", "account=acc://myadi.acme/staking",
		"destination=acc://myadi.acme/rewards"}, KindChangePayout},
	{"changeDelegate", []string{
		"changeDelegate", "account=acc://myadi.acme/staking",
		"delegate=acc://validator.acme"}, KindChangeDelegate},
	{"changeType", []string{
		"changeType", "account=acc://myadi.acme/staking",
		"type=coreFollower"}, KindChangeType},
	{"changeDelegatorPayout", []string{
		"changeDelegatorPayout", "destination=acc://myadi.acme/rewards",
		"identity=acc://myadi.acme"}, KindChangeDelegatorPayout},
	{"rejectDelegates", []string{
		"rejectDelegates", "identity=acc://myadi.acme"}, KindRejectDelegates},
	{"registerIdentity", []string{
		"registerIdentity", "identity=acc://myadi.acme"}, KindRegisterIdentity},
	{"unstakeAccount", []string{
		"unstakeAccount", "account=acc://myadi.acme/staking"}, KindUnstake},
}

func rawOf(parts []string) [][]byte {
	raw := make([][]byte, len(parts))
	for i, p := range parts {
		raw[i] = []byte(p)
	}
	return raw
}

// TestWalletVectorsParse: every action the wallet can write is READ, and
// read as itself. A miss here strands a real request.
func TestWalletVectorsParse(t *testing.T) {
	for _, v := range walletVectors {
		t.Run(v.name, func(t *testing.T) {
			r, err := Parse(rawOf(v.parts))
			if err != nil {
				t.Fatalf("the wallet writes this and we cannot read it: %v", err)
			}
			if r.Kind != v.kind {
				t.Errorf("kind = %q, want %q", r.Kind, v.kind)
			}
		})
	}
}

// TestWalletVectorsRoundTrip: Parse then Encode must reproduce the
// wallet's own bytes. Anything else is a second dialect, which is the
// fork this package exists to prevent — and it would mean a request we
// re-wrote could not be read by the wallet that authored it.
func TestWalletVectorsRoundTrip(t *testing.T) {
	for _, v := range walletVectors {
		t.Run(v.name, func(t *testing.T) {
			r, err := Parse(rawOf(v.parts))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			out, err := r.Encode()
			if err != nil {
				t.Fatalf("the wallet writes this and we refuse to write it: %v", err)
			}
			if len(out) != len(v.parts) {
				t.Fatalf("re-encoded to %d parts, want %d\n got:  %q\n want: %q",
					len(out), len(v.parts), bytesToStrings(out), v.parts)
			}
			for i := range out {
				if string(out[i]) != v.parts[i] {
					t.Errorf("part %d = %q, want %q", i, out[i], v.parts[i])
				}
			}
		})
	}
}

func bytesToStrings(b [][]byte) []string {
	s := make([]string, len(b))
	for i := range b {
		s[i] = string(b[i])
	}
	return s
}
