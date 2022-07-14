package protocol_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestFee(t *testing.T) {
	t.Run("Unknown", func(t *testing.T) {
		_, err := BaseTransactionFee(TransactionTypeUnknown)
		require.Error(t, err)
	})

	t.Run("Invalid", func(t *testing.T) {
		_, err := BaseTransactionFee(TransactionType(math.MaxUint64))
		require.Error(t, err)
	})

	t.Run("SendTokens", func(t *testing.T) {
		env := acctesting.NewTransaction().
			WithCurrentTimestamp().
			WithPrincipal(AcmeUrl()).
			WithSigner(AccountUrl("foo", "book", "1"), 1).
			WithCurrentTimestamp().
			WithBody(new(SendTokens)).
			Initiate(SignatureTypeLegacyED25519, acctesting.GenerateKey(t.Name()))
		fee, err := ComputeTransactionFee(env.Transaction[0])
		require.NoError(t, err)
		require.Equal(t, FeeSendTokens, fee)
	})

	t.Run("Lots of data", func(t *testing.T) {
		env := acctesting.NewTransaction().
			WithPrincipal(AcmeUrl()).
			WithSigner(AccountUrl("foo", "book", "1"), 1).
			WithCurrentTimestamp().
			WithBody(new(SendTokens)).
			Initiate(SignatureTypeLegacyED25519, acctesting.GenerateKey(t.Name()))
		env.Transaction[0].Header.Metadata = make([]byte, 1024)
		fee, err := ComputeTransactionFee(env.Transaction[0])
		require.NoError(t, err)
		require.Equal(t, FeeSendTokens+FeeData*4, fee)
	})

	t.Run("Scratch data", func(t *testing.T) {
		env := acctesting.NewTransaction().
			WithPrincipal(AcmeUrl()).
			WithSigner(AccountUrl("foo", "book", "1"), 1).
			WithCurrentTimestamp().
			WithBody(&WriteData{Scratch: true}).
			Initiate(SignatureTypeLegacyED25519, acctesting.GenerateKey(t.Name()))
		env.Transaction[0].Header.Metadata = make([]byte, 1024)
		fee, err := ComputeTransactionFee(env.Transaction[0])
		require.NoError(t, err)
		require.Equal(t, FeeScratchData*5, fee)
	})
}
