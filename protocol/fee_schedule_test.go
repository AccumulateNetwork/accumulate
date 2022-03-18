package protocol_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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
			WithPrincipal(protocol.AcmeUrl()).
			WithSigner(url.MustParse("foo/book/1"), 1).
			WithNonceTimestamp().
			WithBody(new(protocol.SendTokens)).
			Initiate(SignatureTypeLegacyED25519, acctesting.GenerateKey(t.Name()))
		fee, err := ComputeTransactionFee(env)
		require.NoError(t, err)
		require.Equal(t, protocol.FeeSendTokens, fee)
	})

	t.Run("Lots of data", func(t *testing.T) {
		env := acctesting.NewTransaction().
			WithPrincipal(protocol.AcmeUrl()).
			WithSigner(url.MustParse("foo/book/1"), 1).
			WithNonceTimestamp().
			WithBody(new(protocol.SendTokens)).
			Initiate(SignatureTypeLegacyED25519, acctesting.GenerateKey(t.Name()))
		env.Transaction.Header.Metadata = make([]byte, 1024)
		fee, err := ComputeTransactionFee(env)
		require.NoError(t, err)
		require.Equal(t, protocol.FeeSendTokens+protocol.FeeWriteData*4, fee)
	})
}
