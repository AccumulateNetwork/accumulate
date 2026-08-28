// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package requests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func parts(ss ...string) [][]byte {
	out := make([][]byte, len(ss))
	for i, s := range ss {
		out[i] = []byte(s)
	}
	return out
}

// Every era that exists on chain. These shapes were acted on by the pipeline of
// their day, so failing to read them means telling a staker their own history
// was malformed.
func TestParseLegacyEras(t *testing.T) {
	cases := []struct {
		name  string
		parts [][]byte
		want  Request
	}{
		{
			// Exactly what the wallet emitted before the contract: an action
			// marker followed by its fields, sorted. 12500000000 base units is
			// 125 ACME.
			name:  "wallet's pre-contract withdrawTokens",
			parts: parts("withdrawTokens", "account=acc://alice.acme/staking", "amount=12500000000", "recipient=acc://bob.acme/tokens"),
			want: Request{
				Kind: KindWithdraw, Account: "acc://alice.acme/staking",
				Amount: "125", Destination: "acc://bob.acme/tokens", Era: EraLegacy, Payloads: 1,
			},
		},
		{
			name:  "wallet's pre-contract addAccount",
			parts: parts("addAccount", "account=acc://alice.acme/staking", "payout=acc://alice.acme/rewards", "type=pure"),
			want: Request{
				Kind: KindRegister, Stake: "acc://alice.acme/staking",
				Type: "pure", Rewards: "acc://alice.acme/rewards", Era: EraLegacy, Payloads: 1,
			},
		},
		{
			name:  "oldest era spelled the action withdraw",
			parts: parts("withdraw", "stake=acc://alice.acme/staking", "amount=3,950,000", "destination=acc://bob.acme/tokens"),
			want: Request{
				Kind: KindWithdraw, Account: "acc://alice.acme/staking",
				Amount: "3950000", Destination: "acc://bob.acme/tokens", Era: EraLegacy, Payloads: 1,
			},
		},
		{
			// The oldest registrations carry no action marker at all.
			name:  "registration inferred from stake plus type",
			parts: parts("identity=acc://alice.acme", "stake=acc://alice.acme/staking", "type=coreValidator", "rewards=acc://alice.acme/rewards"),
			want: Request{
				Kind: KindRegister, Stake: "acc://alice.acme/staking", Type: "coreValidator",
				Rewards: "acc://alice.acme/rewards", Identity: "acc://alice.acme", Era: EraLegacy, Payloads: 1,
			},
		},
		{
			name:  "registration inferred from stake plus delegate",
			parts: parts("staking_account=acc://alice.acme/staking", "delegate_to=acc://validator.acme/staking"),
			want: Request{
				Kind: KindRegister, Stake: "acc://alice.acme/staking",
				Delegate: "acc://validator.acme/staking", Era: EraLegacy, Payloads: 1,
			},
		},
		{
			name:  "actionType carried as a field rather than a marker",
			parts: parts("actionType=addAccount", "stakingAccount=acc://alice.acme/staking", "awards=acc://alice.acme/rewards", "type=delegated", "delegate=acc://validator.acme/staking"),
			want: Request{
				Kind: KindRegister, Stake: "acc://alice.acme/staking", Type: "delegated",
				Rewards: "acc://alice.acme/rewards", Delegate: "acc://validator.acme/staking", Era: EraLegacy, Payloads: 1,
			},
		},
		{
			name:  "quoted values and a cross-referenced transaction",
			parts: parts("addAccount", `account="acc://alice.acme/staking"`, "type=“pure”", "request_txid=abc123"),
			want: Request{
				Kind: KindRegister, Stake: "acc://alice.acme/staking", Type: "pure",
				RequestTx: "abc123", Era: EraLegacy, Payloads: 1,
			},
		},
		{
			// An era formatted nil pointers into the entry instead of omitting
			// the field. "<nil>" must not become a delegate URL.
			name:  "nil-formatted fields are ignored",
			parts: parts("addAccount", "account=acc://alice.acme/staking", "type=pure", "delegate=<nil>", "payout=<nil>"),
			want: Request{
				Kind: KindRegister, Stake: "acc://alice.acme/staking", Type: "pure", Era: EraLegacy, Payloads: 1,
			},
		},
		{
			name:  "base-unit amount is converted to ACME",
			parts: parts("withdrawTokens", "account=acc://alice.acme/staking", "amount=10000000000000", "recipient=acc://bob.acme/tokens"),
			want: Request{
				Kind: KindWithdraw, Account: "acc://alice.acme/staking",
				Amount: "100000", Destination: "acc://bob.acme/tokens", Era: EraLegacy, Payloads: 1,
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.parts)
			require.NoError(t, err)
			require.Equal(t, &tt.want, got)
		})
	}
}

// The normalised form has to be lossless across eras, or the shared package
// just moves the divergence up a level: the same intent expressed in the old
// encoding and the new one must produce the same value.
func TestErasNormaliseToTheSameRequest(t *testing.T) {
	t.Run("withdraw", func(t *testing.T) {
		legacy, err := Parse(parts("withdrawTokens",
			"account=acc://alice.acme/staking", "amount=12550000000", "recipient=acc://bob.acme/tokens"))
		require.NoError(t, err)

		contract, err := Parse(parts(
			`{"actionType":"withdraw","account":"acc://alice.acme/staking","destination":"acc://bob.acme/tokens","amount":"125.5"}`))
		require.NoError(t, err)

		require.Equal(t, contract.Kind, legacy.Kind)
		require.Equal(t, contract.Account, legacy.Account)
		require.Equal(t, contract.Destination, legacy.Destination)
		require.Equal(t, contract.Amount, legacy.Amount, "base units and ACME must land on the same value")
		require.Equal(t, contract.Subject(), legacy.Subject())
	})

	t.Run("register", func(t *testing.T) {
		legacy, err := Parse(parts("addAccount",
			"account=acc://alice.acme/staking", "type=delegated", "delegate=acc://validator.acme/staking", "payout=acc://alice.acme/rewards"))
		require.NoError(t, err)

		contract, err := Parse(parts(
			`{"actionType":"register","type":"delegated","stake":"acc://alice.acme/staking","rewards":"acc://alice.acme/rewards","delegate":"acc://validator.acme/staking"}`))
		require.NoError(t, err)

		require.Equal(t, contract.Kind, legacy.Kind)
		require.Equal(t, contract.Stake, legacy.Stake)
		require.Equal(t, contract.Type, legacy.Type)
		require.Equal(t, contract.Rewards, legacy.Rewards)
		require.Equal(t, contract.Delegate, legacy.Delegate)
		require.Equal(t, contract.Subject(), legacy.Subject())
	})
}

// Re-encoding a legacy request migrates it to the contract. This is one-way by
// design and worth pinning, so nobody builds on an assumption of byte-identical
// round-tripping.
func TestLegacyReEncodesToTheWalletForm(t *testing.T) {
	r, err := Parse(parts("addAccount",
		"identity=acc://alice.acme", "account=acc://alice.acme/staking", "type=pure", "request_txid=abc123"))
	require.NoError(t, err)
	require.Equal(t, EraLegacy, r.Era)
	require.Equal(t, "acc://alice.acme", r.Identity)

	out, err := r.Encode()
	require.NoError(t, err)

	// Re-encoding normalises to the WALLET's form: the action marker plus
	// its defined fields, sorted. Identity and the transaction
	// cross-reference are dropped — neither is a field of addAccount
	// (core/wallet requests_types.yml), and the fleet derives the identity
	// from the account.
	//
	// This test formerly asserted a single JSON payload. That was the
	// contract-era encoding, which nothing writes: the wallet has no JSON
	// encoder, so a request re-encoded that way could not be read by the
	// tool that authored it.
	got := make([]string, len(out))
	for i, p := range out {
		got[i] = string(p)
	}
	require.Equal(t, []string{
		"addAccount",
		"account=acc://alice.acme/staking",
		"type=pure",
	}, got)
}

func TestNormalizeAmount(t *testing.T) {
	cases := map[string]string{
		"500024.5136254": "500024.5136254",
		"3,950,000":      "3950000",
		"10000000000000": "100000",
		"100000000":      "100000000", // 9 digits: too short to be base units
		"12345678901":    "123.45678901",
		"  42  ":         "42",
		"":               "",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			require.Equal(t, want, NormalizeAmount(in))
		})
	}
}

func TestCanonicalClass(t *testing.T) {
	for _, in := range []string{"coreValidator", "corevalidator", "CoreValidator", "core-validator", "  core-validator  "} {
		got, ok := CanonicalClass(in)
		require.Truef(t, ok, "%q should be a class", in)
		require.Equal(t, "coreValidator", got)
	}
	for _, in := range []string{"validator", "", "stakingFollower", "core validator"} {
		_, ok := CanonicalClass(in)
		require.Falsef(t, ok, "%q must not be a class", in)
	}
}

// The base-unit heuristic has a blind spot, and it is inherited deliberately.
//
// core/staking treats an integer of 11+ digits as base units, on the reasoning
// that no single withdrawal approaches 10 billion ACME. Below that it reads the
// integer as ACME. So a pre-contract withdrawal of under 100 ACME — which the
// wallet wrote in base units — is indistinguishable from a much larger ACME
// amount, and normalises to the larger reading.
//
// This is pinned rather than fixed. The staking side owns the heuristic, both
// readers must agree, and no amount of local cleverness can recover
// information the encoding never carried. Anyone tempted to "fix" it here
// should change it there first.
func TestBaseUnitHeuristicBlindSpot(t *testing.T) {
	// 10 ACME in base units is 10 digits, under the threshold.
	require.Equal(t, "1000000000", NormalizeAmount("1000000000"),
		"10 ACME written in base units reads as 1,000,000,000 ACME — the encoding cannot tell them apart")

	// One digit more and the heuristic engages.
	require.Equal(t, "100", NormalizeAmount("10000000000"))
}
