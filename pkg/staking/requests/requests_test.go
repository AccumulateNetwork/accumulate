package requests

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// The fleet re-reads the entry off the chain and re-derives the request from
// these exact bytes, so the encoding is a cross-repo contract: core/staking
// cmd/asp/requests.go (verifyRequestOnChain) and pkg/genbrowser/requests.go
// must parse precisely what Encode produces. Changing a vector means the
// staking system has to change with it.
func TestEncodeGoldenVectors(t *testing.T) {
	cases := []struct {
		name  string
		build func() (*Request, error)
		want  string
	}{
		{
			name: "withdraw to an outside account",
			build: func() (*Request, error) {
				return Withdraw("acc://myadi.acme/staking", "acc://someone.acme/tokens", "12.5")
			},
			want: `{"actionType":"withdraw","account":"acc://myadi.acme/staking","destination":"acc://someone.acme/tokens","amount":"12.5"}`,
		},
		{
			name: "withdraw to another staking account",
			build: func() (*Request, error) {
				return Withdraw("acc://myadi.acme/staking", "acc://validator.acme/staking", "1000")
			},
			want: `{"actionType":"withdraw","account":"acc://myadi.acme/staking","destination":"acc://validator.acme/staking","amount":"1000"}`,
		},
		{
			name:  "register pure",
			build: func() (*Request, error) { return Register("acc://myadi.acme/staking", "pure", "", "") },
			want:  `{"actionType":"register","type":"pure","stake":"acc://myadi.acme/staking"}`,
		},
		{
			name: "register with a payout destination",
			build: func() (*Request, error) {
				return Register("acc://myadi.acme/staking", "pure", "acc://myadi.acme/rewards", "")
			},
			want: `{"actionType":"register","type":"pure","stake":"acc://myadi.acme/staking","rewards":"acc://myadi.acme/rewards"}`,
		},
		{
			name: "register delegated",
			build: func() (*Request, error) {
				return Register("acc://myadi.acme/staking", "delegated", "", "acc://validator.acme/staking")
			},
			want: `{"actionType":"register","type":"delegated","stake":"acc://myadi.acme/staking","delegate":"acc://validator.acme/staking"}`,
		},
		{
			// The hyphenated form a user types is canonicalised on the way out;
			// the fleet compares case-insensitively but the entry should read
			// the way the spec spells it.
			name:  "register core-validator canonicalises",
			build: func() (*Request, error) { return Register("acc://myadi.acme/staking", "core-validator", "", "") },
			want:  `{"actionType":"register","type":"coreValidator","stake":"acc://myadi.acme/staking"}`,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			r, err := tt.build()
			require.NoError(t, err)

			parts, err := r.Encode()
			require.NoError(t, err)
			require.Len(t, parts, 1, "one entry, one request")
			require.Equal(t, tt.want, string(parts[0]))

			back, err := Parse(parts)
			require.NoError(t, err)
			require.Equal(t, EraContract, back.Era)
			require.Equal(t, r.Kind, back.Kind)
			require.Equal(t, r.Subject(), back.Subject())
		})
	}
}

func TestAmountSyntax(t *testing.T) {
	accepted := []string{"1", "12.5", "1000", "0.00000001", "999999999999", "12.50000000"}
	for _, a := range accepted {
		t.Run("accepts "+a, func(t *testing.T) {
			_, err := Withdraw("acc://a.acme/s", "acc://b.acme/t", a)
			require.NoError(t, err)
		})
	}

	// A general number parser accepts every one of these and none of them reads
	// as the amount that would move, which is why the fleet refuses them.
	rejected := []string{"1e9", "0x2710", "1/3", "12.5 ACME", "-5", "", ".5", "1.123456789", "1234567890123"}
	for _, a := range rejected {
		t.Run("rejects "+a, func(t *testing.T) {
			_, err := Withdraw("acc://a.acme/s", "acc://b.acme/t", a)
			require.ErrorContains(t, err, "plain decimal")
		})
	}
}

func TestRegisterValidation(t *testing.T) {
	t.Run("rejects a class the signers refuse", func(t *testing.T) {
		_, err := Register("acc://a.acme/s", "validator", "", "")
		require.ErrorContains(t, err, "not a staking class")
	})
	t.Run("rejects delegated without a delegate", func(t *testing.T) {
		_, err := Register("acc://a.acme/s", "delegated", "", "")
		require.ErrorContains(t, err, "needs a delegate")
	})
	t.Run("accepts every documented class", func(t *testing.T) {
		for _, c := range Classes {
			delegate := ""
			if c == "delegated" {
				delegate = "acc://v.acme/s"
			}
			_, err := Register("acc://a.acme/s", c, "", delegate)
			require.NoErrorf(t, err, "class %q must be accepted", c)
		}
	})
}

// Validate must not refuse a request that is merely early. The full-stake
// minimum and "the delegate is not yet a registered staker" are governance
// gates: the request is well formed, the signers re-check it every pass, and it
// starts working once the precondition is met without being refiled.
func TestValidateDoesNotApplyGovernanceGates(t *testing.T) {
	r, err := Register("acc://tiny.acme/staking", "coreValidator", "", "")
	require.NoError(t, err, "a validator registration is valid regardless of balance")

	r, err = Register("acc://a.acme/staking", "delegated", "", "acc://not-registered.acme/staking")
	require.NoError(t, err, "an unregistered delegate is a governance matter, not a malformed request")
	require.Equal(t, "acc://not-registered.acme/staking", r.Delegate)
}

func TestNotARequest(t *testing.T) {
	cases := []struct {
		name  string
		parts [][]byte
	}{
		{"binary blob", [][]byte{{0x00, 0x01, 0x02}}},
		{"free text announcement", [][]byte{[]byte("staking rewards are paid weekly")}},
		{"empty entry", [][]byte{}},
		{"empty payload", [][]byte{[]byte("")}},
		{"a test entry that leads with nothing recognisable", [][]byte{[]byte("test"), []byte("foo=bar")}},
		{"JSON with an unknown actionType", [][]byte{[]byte(`{"actionType":"transferTokens","account":"acc://a.acme/s"}`)}},
		{"fields but no action and no type or delegate", [][]byte{[]byte("account=acc://a.acme/s")}},
		{"withdraw notice with no amount", [][]byte{[]byte("withdrawTokens"), []byte("account=acc://a.acme/s")}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.parts)
			require.ErrorIs(t, err, ErrNotARequest)
		})
	}
}

// Parsing recognises a shape; validation judges it. Keeping them separate lets
// a caller display a badly-formed request AND say what is wrong with it,
// instead of showing nothing.
func TestParseAcceptsWhatValidateRejects(t *testing.T) {
	parts := [][]byte{[]byte(`{"actionType":"withdraw","account":"acc://a.acme/s","destination":"acc://b.acme/t","amount":"1e9"}`)}

	r, err := Parse(parts)
	require.NoError(t, err, "the shape is a withdrawal, however bad the amount")
	require.Equal(t, KindWithdraw, r.Kind)
	require.ErrorContains(t, r.Validate(), "plain decimal")
}

// Several contract payloads in one entry is a refusal — they would share an
// entry hash and so a fulfillment memo — but it is still the contract era and
// should not fall through to the legacy reader.
func TestMultiPayloadEntryIsContractEra(t *testing.T) {
	one := []byte(`{"actionType":"withdraw","account":"acc://a.acme/s","destination":"acc://b.acme/t","amount":"1"}`)

	r, err := Parse([][]byte{one, one})
	require.NoError(t, err)
	require.Equal(t, EraContract, r.Era)
	require.Equal(t, KindWithdraw, r.Kind)
}

func TestErrNotARequestIsMatchable(t *testing.T) {
	_, err := Parse([][]byte{[]byte("hello")})
	require.True(t, errors.Is(err, ErrNotARequest))
}
