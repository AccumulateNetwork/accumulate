package protocol_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestFee(t *testing.T) {
	t.Run("SendTokens", func(t *testing.T) {
		env := acctesting.NewTransaction().
			WithCurrentTimestamp().
			WithPrincipal(AcmeUrl()).
			WithSigner(AccountUrl("foo", "book", "1"), 1).
			WithCurrentTimestamp().
			WithBody(&SendTokens{To: []*TokenRecipient{{}}}).
			Initiate(SignatureTypeLegacyED25519, acctesting.GenerateKey(t.Name()))
		fee, err := ComputeTransactionFee(env.Transaction[0])
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
		fee, err := ComputeTransactionFee(env.Transaction[0])
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
		fee, err := ComputeTransactionFee(env.Transaction[0])
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
				fee, err := ComputeTransactionFee(txn)
				if i == TransactionTypeRemote {
					// Remote transactions should never be passed to
					// ComputeTransactionFee so it returns an error
					require.Error(t, err)
				} else {
					// Every other legal transaction type must have a fee
					require.NoError(t, err)
				}

				switch i {
				case TransactionTypeAcmeFaucet,
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
