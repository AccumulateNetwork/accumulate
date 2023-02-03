// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol_test

import (
	"math/big"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestFee(t *testing.T) {
	s := new(FeeSchedule)
	t.Run("SendTokens", func(t *testing.T) {
		env := acctesting.NewTransaction().
			WithCurrentTimestamp().
			WithPrincipal(AcmeUrl()).
			WithSigner(AccountUrl("foo", "book", "1"), 1).
			WithCurrentTimestamp().
			WithBody(&SendTokens{To: []*TokenRecipient{{}}}).
			Initiate(SignatureTypeLegacyED25519, acctesting.GenerateKey(t.Name()))
		fee, err := s.ComputeTransactionFee(env.Transaction[0])
		require.NoError(t, err)
		require.Equal(t, FeeTransferTokens, fee)
	})

	t.Run("Lots of data", func(t *testing.T) {
		env := acctesting.NewTransaction().
			WithPrincipal(AcmeUrl()).
			WithSigner(AccountUrl("foo", "book", "1"), 1).
			WithCurrentTimestamp().
			WithBody(&SendTokens{To: []*TokenRecipient{{}}}).
			Initiate(SignatureTypeLegacyED25519, acctesting.GenerateKey(t.Name()))
		env.Transaction[0].Header.Metadata = make([]byte, 1024)
		fee, err := s.ComputeTransactionFee(env.Transaction[0])
		require.NoError(t, err)
		require.Equal(t, FeeTransferTokens+FeeData*4, fee)
	})

	t.Run("Scratch data", func(t *testing.T) {
		env := acctesting.NewTransaction().
			WithPrincipal(AcmeUrl()).
			WithSigner(AccountUrl("foo", "book", "1"), 1).
			WithCurrentTimestamp().
			WithBody(&WriteData{Scratch: true}).
			Initiate(SignatureTypeLegacyED25519, acctesting.GenerateKey(t.Name()))
		env.Transaction[0].Header.Metadata = make([]byte, 1024)
		fee, err := s.ComputeTransactionFee(env.Transaction[0])
		require.NoError(t, err)
		require.Equal(t, FeeScratchData*5, fee)
	})

	t.Run("All types", func(t *testing.T) {
		// Test every valid user transaction
		for i := TransactionType(0); i < 1<<16; i++ {
			body, err := NewTransactionBody(i)
			if err != nil {
				continue
			}

			t.Run(i.String(), func(t *testing.T) {
				txn := new(Transaction)
				txn.Header.Principal = AccountUrl("foo")
				txn.Body = body
				fee, err := s.ComputeTransactionFee(txn)
				if i == TransactionTypeRemote {
					// Remote transactions should never be passed to
					// ComputeTransactionFee so it returns an error
					require.Error(t, err)
					return
				} else {
					// Every other legal transaction type must have a fee
					require.NoError(t, err)
				}

				switch i {
				case TransactionTypeActivateProtocolVersion,
					TransactionTypeAcmeFaucet,
					TransactionTypePlaceholder,
					TransactionTypeAddCredits:
					// A few transactions are free
					require.Zero(t, fee)
				default:
					if i.IsUser() {
						// Most user transactions should cost something
						require.NotZero(t, fee)
					} else {
						// Synthetic and system transactions are free
						require.Zero(t, fee)
					}
				}
			})
		}
	})
}

func TestSubAdiFee(t *testing.T) {
	s := new(FeeSchedule)
	s.CreateIdentitySliding = []Fee{
		FeeCreateIdentity << 12,
		FeeCreateIdentity << 11,
		FeeCreateIdentity << 10,
		FeeCreateIdentity << 9,
		FeeCreateIdentity << 8,
		FeeCreateIdentity << 7,
		FeeCreateIdentity << 6,
		FeeCreateIdentity << 5,
		FeeCreateIdentity << 4,
		FeeCreateIdentity << 3,
		FeeCreateIdentity << 2,
		FeeCreateIdentity << 1,
	}

	env := acctesting.NewTransaction().
		WithCurrentTimestamp().
		WithPrincipal(AcmeUrl()).
		WithSigner(AccountUrl("foo", "book", "1"), 1).
		WithCurrentTimestamp().
		WithBody(&CreateIdentity{Url: AccountUrl("foo.acme", "sub")}).
		Initiate(SignatureTypeLegacyED25519, acctesting.GenerateKey(t.Name()))
	fee, err := s.ComputeTransactionFee(env.Transaction[0])
	require.NoError(t, err)

	require.Equal(t, FeeCreateIdentity, fee)
}

func TestSlidingIdentityFeeSchedule(t *testing.T) {
	s := new(FeeSchedule)
	s.CreateIdentitySliding = []Fee{
		FeeCreateIdentity << 12,
		FeeCreateIdentity << 11,
		FeeCreateIdentity << 10,
		FeeCreateIdentity << 9,
		FeeCreateIdentity << 8,
		FeeCreateIdentity << 7,
		FeeCreateIdentity << 6,
		FeeCreateIdentity << 5,
		FeeCreateIdentity << 4,
		FeeCreateIdentity << 3,
		FeeCreateIdentity << 2,
		FeeCreateIdentity << 1,
	}

	for i := 0; i <= len(s.CreateIdentitySliding); i++ {
		env := acctesting.NewTransaction().
			WithCurrentTimestamp().
			WithPrincipal(AcmeUrl()).
			WithSigner(AccountUrl("foo", "book", "1"), 1).
			WithCurrentTimestamp().
			WithBody(&CreateIdentity{Url: AccountUrl(strings.Repeat("a", i+1))}).
			Initiate(SignatureTypeLegacyED25519, acctesting.GenerateKey(t.Name()))
		fee, err := s.ComputeTransactionFee(env.Transaction[0])
		require.NoError(t, err)

		if i < len(s.CreateIdentitySliding) {
			require.Equal(t, s.CreateIdentitySliding[i], fee)
		} else {
			require.Equal(t, FeeCreateIdentity, fee)
		}
	}
}

func TestMultiOutputRefund(t *testing.T) {
	// Verifies that a two output transaction with a good and a bad output does
	// not cost less than a single output transaction

	s := new(FeeSchedule)
	body := new(SendTokens)
	txn := new(Transaction)
	txn.Header.Principal = AccountUrl("foo")
	txn.Body = body

	// Calculate fee and refund for a single output transaction
	body.AddRecipient(AccountUrl("bar"), big.NewInt(0))
	fee1, err := s.ComputeTransactionFee(txn)
	require.NoError(t, err)

	// Calculate fee and refund for a two output transaction
	body.AddRecipient(AccountUrl("bar"), big.NewInt(0))
	paid2, err := s.ComputeTransactionFee(txn)
	require.NoError(t, err)
	refund2, err := s.ComputeSyntheticRefund(txn, len(body.To))
	require.NoError(t, err)
	fee2 := paid2 - refund2

	// Verify
	require.GreaterOrEqual(t, fee2, fee1)
}
