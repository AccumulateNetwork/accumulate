// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol_test

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestFee(t *testing.T) {
	t.Run("SendTokens", func(t *testing.T) {
		s := new(FeeSchedule)

		env :=
			MustBuild(t, build.Transaction().
				For(AcmeUrl()).
				Body(&SendTokens{To: []*TokenRecipient{{}}}).
				SignWith(AccountUrl("foo", "book", "1")).Version(1).Timestamp(time.Now()).PrivateKey(acctesting.GenerateKey(t.Name())).Type(SignatureTypeLegacyED25519))

		fee, err := s.ComputeTransactionFee(env.Transaction[0])
		require.NoError(t, err)
		require.Equal(t, FeeTransferTokens, fee)
	})

	t.Run("Lots of data", func(t *testing.T) {
		s := new(FeeSchedule)

		env :=
			MustBuild(t, build.Transaction().
				For(AcmeUrl()).
				Body(&SendTokens{To: []*TokenRecipient{{}}}).
				SignWith(AccountUrl("foo", "book", "1")).Version(1).Timestamp(time.Now()).PrivateKey(acctesting.GenerateKey(t.Name())).Type(SignatureTypeLegacyED25519))

		env.Transaction[0].Header.Metadata = make([]byte, 1024)
		fee, err := s.ComputeTransactionFee(env.Transaction[0])
		require.NoError(t, err)
		require.Equal(t, FeeTransferTokens+FeeData*4, fee)
	})

	t.Run("Scratch data", func(t *testing.T) {
		s := new(FeeSchedule)

		env :=
			MustBuild(t, build.Transaction().
				For(AcmeUrl()).
				Body(&WriteData{Scratch: true}).
				SignWith(AccountUrl("foo", "book", "1")).Version(1).Timestamp(time.Now()).PrivateKey(acctesting.GenerateKey(t.Name())).Type(SignatureTypeLegacyED25519))

		env.Transaction[0].Header.Metadata = make([]byte, 1024)
		fee, err := s.ComputeTransactionFee(env.Transaction[0])
		require.NoError(t, err)
		require.Equal(t, FeeScratchData*5, fee)
	})

	t.Run("Create root ADI", func(t *testing.T) {
		s := new(FeeSchedule)

		// Make sure this is ignored for a root identity
		s.CreateSubIdentity = 100

		env :=
			MustBuild(t, build.Transaction().
				For(AcmeUrl()).
				CreateIdentity("foo").
				SignWith(AccountUrl("foo", "book", "1")).Version(1).Timestamp(time.Now()).PrivateKey(acctesting.GenerateKey(t.Name())).Type(SignatureTypeLegacyED25519))

		fee, err := s.ComputeTransactionFee(env.Transaction[0])
		require.NoError(t, err)
		require.Equal(t, FeeCreateIdentity, fee)
	})

	t.Run("Create sub ADI (original)", func(t *testing.T) {
		s := new(FeeSchedule)

		env :=
			MustBuild(t, build.Transaction().
				For(AcmeUrl()).
				CreateIdentity("foo", "bar").
				SignWith(AccountUrl("foo", "book", "1")).Version(1).Timestamp(time.Now()).PrivateKey(acctesting.GenerateKey(t.Name())).Type(SignatureTypeLegacyED25519))

		fee, err := s.ComputeTransactionFee(env.Transaction[0])
		require.NoError(t, err)
		require.Equal(t, FeeCreateIdentity, fee)
	})

	t.Run("Create sub ADI (reduced)", func(t *testing.T) {
		s := new(FeeSchedule)
		s.CreateSubIdentity = 100

		env :=
			MustBuild(t, build.Transaction().
				For(AcmeUrl()).
				CreateIdentity("foo", "bar").
				SignWith(AccountUrl("foo", "book", "1")).Version(1).Timestamp(time.Now()).PrivateKey(acctesting.GenerateKey(t.Name())).Type(SignatureTypeLegacyED25519))

		fee, err := s.ComputeTransactionFee(env.Transaction[0])
		require.NoError(t, err)
		require.Equal(t, s.CreateSubIdentity, fee)
	})

	t.Run("All types", func(t *testing.T) {
		s := new(FeeSchedule)

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
					TransactionTypeBurnCredits,
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

	env :=
		MustBuild(t, build.Transaction().
			For(AcmeUrl()).
			Body(&CreateIdentity{Url: AccountUrl("foo.acme", "sub")}).
			SignWith(AccountUrl("foo", "book", "1")).Version(1).Timestamp(time.Now()).PrivateKey(acctesting.GenerateKey(t.Name())).Type(SignatureTypeLegacyED25519))

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
		env :=
			MustBuild(t, build.Transaction().
				For(AcmeUrl()).
				Body(&CreateIdentity{Url: AccountUrl(strings.Repeat("a", i+1))}).
				SignWith(AccountUrl("foo", "book", "1")).Version(1).Timestamp(time.Now()).PrivateKey(acctesting.GenerateKey(t.Name())).Type(SignatureTypeLegacyED25519))

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
