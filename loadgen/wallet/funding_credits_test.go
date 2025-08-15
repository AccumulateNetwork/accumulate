package wallet

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	errors2 "gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// MockQueryClient is a mock implementation of QueryClient
type MockQueryClient struct {
	mock.Mock
}

func (m *MockQueryClient) Query(ctx context.Context, u *url.URL, query api.Query) (api.Record, error) {
	args := m.Called(ctx, u, query)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(api.Record), args.Error(1)
}

// MockSubmitClient is a mock implementation of SubmitClient
type MockSubmitClient struct {
	mock.Mock
}

func (m *MockSubmitClient) Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	args := m.Called(ctx, envelope, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*api.Submission), args.Error(1)
}

// MockTransactionSigner is a mock implementation of TransactionSigner
type MockTransactionSigner struct {
	mock.Mock
}

func (m *MockTransactionSigner) SignTransaction(txn *protocol.Transaction, signerUrl *url.URL, privateKey []byte) (*messaging.Envelope, error) {
	args := m.Called(txn, signerUrl, privateKey)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*messaging.Envelope), args.Error(1)
}

// Helper function to create test URLs
func mustParseURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

// TestCreditManager_TopUpLiteAccount tests topping up a lite account
func TestCreditManager_TopUpLiteAccount(t *testing.T) {
	t.Run("successful top-up when balance is low", func(t *testing.T) {
		// Setup mocks
		mockClient := new(MockQueryClient)
		mockSubmitter := new(MockSubmitClient)
		mockSigner := new(MockTransactionSigner)

		// Create funding account
		fundingAccount := &LiteIdentity{
			URL:          mustParseURL("acc://lite-funding.acme/ACME"),
			Key:          &Key{PrivateKey: []byte("test-private-key")},
			CreditBalance: 1000,
		}

		// Create target lite account
		targetAccount := &LiteIdentity{
			URL:          mustParseURL("acc://lite-target.acme/ACME"),
			CreditBalance: 10, // Low balance, should trigger top-up
		}

		// Setup funding account balance query
		fundingTokenAccount := &protocol.TokenAccount{
			Balance: *big.NewInt(1000 * protocol.AcmePrecision), // 1000 ACME
		}
		fundingRecord := &api.AccountRecord{
			Account: fundingTokenAccount,
		}
		mockClient.On("Query", mock.Anything, mock.MatchedBy(func(u *url.URL) bool {
			return u.String() == "acc://lite-funding.acme/ACME"
		}), mock.Anything).
			Return(fundingRecord, nil)

		// Setup target account credits query
		targetLiteIdentity := &protocol.LiteIdentity{
			CreditBalance: 10, // Low credits
		}
		targetRecord := &api.AccountRecord{
			Account: targetLiteIdentity,
		}
		mockClient.On("Query", mock.Anything, mock.MatchedBy(func(u *url.URL) bool {
			return u.String() == targetAccount.URL.String()
		}), mock.Anything).
			Return(targetRecord, nil)

		// Setup transaction signing
		envelope := &messaging.Envelope{}
		mockSigner.On("SignTransaction", mock.Anything, fundingAccount.URL, fundingAccount.Key.PrivateKey).
			Return(envelope, nil)

		// Setup transaction submission
		submissions := []*api.Submission{
			{
				Status: &protocol.TransactionStatus{
					Code: 0, // Success status
				},
			},
		}
		mockSubmitter.On("Submit", mock.Anything, envelope, mock.Anything).
			Return(submissions, nil)

		// Create credit manager and execute
		cm := NewCreditManager(mockClient, mockSubmitter, mockSigner, fundingAccount)
		err := cm.TopUpLiteAccount(context.Background(), targetAccount)

		// Assertions
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
		mockSubmitter.AssertExpectations(t)
		mockSigner.AssertExpectations(t)
	})

	t.Run("skip top-up when balance is sufficient", func(t *testing.T) {
		// Setup mocks
		mockClient := new(MockQueryClient)
		mockSubmitter := new(MockSubmitClient)
		mockSigner := new(MockTransactionSigner)

		// Create funding account
		fundingAccount := &LiteIdentity{
			URL:          mustParseURL("acc://lite-funding.acme/ACME"),
			Key:          &Key{PrivateKey: []byte("test-private-key")},
		}

		// Create target lite account with high balance
		targetAccount := &LiteIdentity{
			URL:          mustParseURL("acc://lite-target.acme/ACME"),
			CreditBalance: 600, // High balance, should skip top-up
		}

		// Setup funding account balance query
		fundingTokenAccount := &protocol.TokenAccount{
			Balance: *big.NewInt(1000 * protocol.AcmePrecision),
		}
		fundingRecord := &api.AccountRecord{
			Account: fundingTokenAccount,
		}
		mockClient.On("Query", mock.Anything, mock.MatchedBy(func(u *url.URL) bool {
			return u.String() == "acc://lite-funding.acme/ACME"
		}), mock.Anything).
			Return(fundingRecord, nil)

		// Setup target account credits query with high balance
		targetLiteIdentity := &protocol.LiteIdentity{
			CreditBalance: 600, // Above MaximumCreditBalance
		}
		targetRecord := &api.AccountRecord{
			Account: targetLiteIdentity,
		}
		mockClient.On("Query", mock.Anything, mock.MatchedBy(func(u *url.URL) bool {
			return u.String() == targetAccount.URL.String()
		}), mock.Anything).
			Return(targetRecord, nil)

		// Create credit manager and execute
		cm := NewCreditManager(mockClient, mockSubmitter, mockSigner, fundingAccount)
		err := cm.TopUpLiteAccount(context.Background(), targetAccount)

		// Assertions - should not error and should not submit transaction
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
		mockSubmitter.AssertNotCalled(t, "Submit")
		mockSigner.AssertNotCalled(t, "SignTransaction")
	})

	t.Run("error when funding account has insufficient balance", func(t *testing.T) {
		// Setup mocks
		mockClient := new(MockQueryClient)
		mockSubmitter := new(MockSubmitClient)
		mockSigner := new(MockTransactionSigner)

		// Create funding account
		fundingAccount := &LiteIdentity{
			URL:        mustParseURL("acc://lite-funding.acme/ACME"),
			Key:        &Key{PrivateKey: []byte("test-private-key")},
		}

		// Create target lite account
		targetAccount := &LiteIdentity{
			URL: mustParseURL("acc://lite-target.acme/ACME"),
		}

		// Setup funding account with low balance
		fundingTokenAccount := &protocol.TokenAccount{
			Balance: *big.NewInt(10 * protocol.AcmePrecision), // Only 10 ACME (below minimum)
		}
		fundingRecord := &api.AccountRecord{
			Account: fundingTokenAccount,
		}
		mockClient.On("Query", mock.Anything, mock.MatchedBy(func(u *url.URL) bool {
			return u.String() == "acc://lite-funding.acme/ACME"
		}), mock.Anything).
			Return(fundingRecord, nil)

		// Create credit manager and execute
		cm := NewCreditManager(mockClient, mockSubmitter, mockSigner, fundingAccount)
		err := cm.TopUpLiteAccount(context.Background(), targetAccount)

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "insufficient funding")
		mockClient.AssertExpectations(t)
		mockSubmitter.AssertNotCalled(t, "Submit")
	})

	t.Run("error when query fails", func(t *testing.T) {
		// Setup mocks
		mockClient := new(MockQueryClient)
		mockSubmitter := new(MockSubmitClient)
		mockSigner := new(MockTransactionSigner)

		// Create funding account
		fundingAccount := &LiteIdentity{
			URL:        mustParseURL("acc://lite-funding.acme/ACME"),
			Key:        &Key{PrivateKey: []byte("test-private-key")},
		}

		// Create target lite account
		targetAccount := &LiteIdentity{
			URL: mustParseURL("acc://lite-target.acme/ACME"),
		}

		// Setup funding account query to fail
		mockClient.On("Query", mock.Anything, mock.MatchedBy(func(u *url.URL) bool {
			return u.String() == "acc://lite-funding.acme/ACME"
		}), mock.Anything).
			Return(nil, errors.New("network error"))

		// Create credit manager and execute
		cm := NewCreditManager(mockClient, mockSubmitter, mockSigner, fundingAccount)
		err := cm.TopUpLiteAccount(context.Background(), targetAccount)

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check funding balance")
		mockClient.AssertExpectations(t)
		mockSubmitter.AssertNotCalled(t, "Submit")
	})

	t.Run("error when transaction submission fails", func(t *testing.T) {
		// Setup mocks
		mockClient := new(MockQueryClient)
		mockSubmitter := new(MockSubmitClient)
		mockSigner := new(MockTransactionSigner)

		// Create funding account
		fundingAccount := &LiteIdentity{
			URL:        mustParseURL("acc://lite-funding.acme/ACME"),
			Key:        &Key{PrivateKey: []byte("test-private-key")},
		}

		// Create target lite account
		targetAccount := &LiteIdentity{
			URL:          mustParseURL("acc://lite-target.acme/ACME"),
			CreditBalance: 10,
		}

		// Setup funding account balance query
		fundingTokenAccount := &protocol.TokenAccount{
			Balance: *big.NewInt(1000 * protocol.AcmePrecision),
		}
		fundingRecord := &api.AccountRecord{
			Account: fundingTokenAccount,
		}
		mockClient.On("Query", mock.Anything, mock.MatchedBy(func(u *url.URL) bool {
			return u.String() == "acc://lite-funding.acme/ACME"
		}), mock.Anything).
			Return(fundingRecord, nil)

		// Setup target account credits query
		targetLiteIdentity := &protocol.LiteIdentity{
			CreditBalance: 10,
		}
		targetRecord := &api.AccountRecord{
			Account: targetLiteIdentity,
		}
		mockClient.On("Query", mock.Anything, mock.MatchedBy(func(u *url.URL) bool {
			return u.String() == targetAccount.URL.String()
		}), mock.Anything).
			Return(targetRecord, nil)

		// Setup transaction signing
		envelope := &messaging.Envelope{}
		mockSigner.On("SignTransaction", mock.Anything, fundingAccount.URL, fundingAccount.Key.PrivateKey).
			Return(envelope, nil)

		// Setup transaction submission to fail
		mockSubmitter.On("Submit", mock.Anything, envelope, mock.Anything).
			Return(nil, errors.New("submission failed"))

		// Create credit manager and execute
		cm := NewCreditManager(mockClient, mockSubmitter, mockSigner, fundingAccount)
		err := cm.TopUpLiteAccount(context.Background(), targetAccount)

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to submit transaction")
		mockClient.AssertExpectations(t)
		mockSubmitter.AssertExpectations(t)
		mockSigner.AssertExpectations(t)
	})
}

// TestCreditManager_TopUpKeyPage tests topping up a key page
func TestCreditManager_TopUpKeyPage(t *testing.T) {
	t.Run("successful key page top-up", func(t *testing.T) {
		// Setup mocks
		mockClient := new(MockQueryClient)
		mockSubmitter := new(MockSubmitClient)
		mockSigner := new(MockTransactionSigner)

		// Create funding account
		fundingAccount := &LiteIdentity{
			URL:        mustParseURL("acc://lite-funding.acme/ACME"),
			Key:        &Key{PrivateKey: []byte("test-private-key")},
		}

		// Create target key page
		targetKeyPage := &KeyPage{
			URL:          mustParseURL("acc://adi.acme/page1"),
			CreditBalance: 50,
		}

		// Setup funding account balance query
		fundingTokenAccount := &protocol.TokenAccount{
			Balance: *big.NewInt(1000 * protocol.AcmePrecision),
		}
		fundingRecord := &api.AccountRecord{
			Account: fundingTokenAccount,
		}
		mockClient.On("Query", mock.Anything, mock.MatchedBy(func(u *url.URL) bool {
			return u.String() == "acc://lite-funding.acme/ACME"
		}), mock.Anything).
			Return(fundingRecord, nil)

		// Setup key page credits query
		keyPageProtocol := &protocol.KeyPage{
			CreditBalance: 50,
		}
		keyPageRecord := &api.AccountRecord{
			Account: keyPageProtocol,
		}
		mockClient.On("Query", mock.Anything, mock.MatchedBy(func(u *url.URL) bool {
			return u.String() == targetKeyPage.URL.String()
		}), mock.Anything).
			Return(keyPageRecord, nil)

		// Setup transaction signing
		envelope := &messaging.Envelope{}
		mockSigner.On("SignTransaction", mock.Anything, fundingAccount.URL, fundingAccount.Key.PrivateKey).
			Return(envelope, nil)

		// Setup transaction submission
		submissions := []*api.Submission{
			{
				Status: &protocol.TransactionStatus{
					Code: 0, // Success status
				},
			},
		}
		mockSubmitter.On("Submit", mock.Anything, envelope, mock.Anything).
			Return(submissions, nil)

		// Create credit manager and execute
		cm := NewCreditManager(mockClient, mockSubmitter, mockSigner, fundingAccount)
		err := cm.TopUpKeyPage(context.Background(), targetKeyPage)

		// Assertions
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
		mockSubmitter.AssertExpectations(t)
		mockSigner.AssertExpectations(t)
	})
}

// TestCreditTargets tests the credit target interfaces
func TestCreditTargets(t *testing.T) {
	t.Run("LiteAccountTarget", func(t *testing.T) {
		lite := &LiteIdentity{
			URL:          mustParseURL("acc://lite.acme/ACME"),
			CreditBalance: 100,
		}
		
		target := &LiteAccountTarget{LiteIdentity: lite}
		
		assert.Equal(t, lite.URL, target.GetURL())
		assert.Equal(t, uint64(100), target.GetCreditBalance())
		assert.Equal(t, "lite", target.GetType())
		
		target.SetCreditBalance(200)
		assert.Equal(t, uint64(200), lite.CreditBalance)
	})

	t.Run("KeyPageTarget", func(t *testing.T) {
		keyPage := &KeyPage{
			URL:          mustParseURL("acc://adi.acme/page1"),
			CreditBalance: 150,
		}
		
		target := &KeyPageTarget{KeyPage: keyPage}
		
		assert.Equal(t, keyPage.URL, target.GetURL())
		assert.Equal(t, uint64(150), target.GetCreditBalance())
		assert.Equal(t, "keypage", target.GetType())
		
		target.SetCreditBalance(300)
		assert.Equal(t, uint64(300), keyPage.CreditBalance)
	})
}

// TestCreditManager_AddCreditsCalculation tests the credit amount calculation
func TestCreditManager_AddCreditsCalculation(t *testing.T) {
	t.Run("verify credit to ACME conversion", func(t *testing.T) {
		// Setup mocks
		mockClient := new(MockQueryClient)
		mockSubmitter := new(MockSubmitClient)
		mockSigner := new(MockTransactionSigner)

		// Create funding account
		fundingAccount := &LiteIdentity{
			URL:        mustParseURL("acc://lite-funding.acme/ACME"),
			Key:        &Key{PrivateKey: []byte("test-private-key")},
		}

		// Create target account
		targetAccount := &LiteIdentity{
			URL: mustParseURL("acc://lite-target.acme/ACME"),
		}

		// Setup funding account balance query
		fundingTokenAccount := &protocol.TokenAccount{
			Balance: *big.NewInt(1000 * protocol.AcmePrecision),
		}
		fundingRecord := &api.AccountRecord{
			Account: fundingTokenAccount,
		}
		mockClient.On("Query", mock.Anything, mock.MatchedBy(func(u *url.URL) bool {
			return u.String() == "acc://lite-funding.acme/ACME"
		}), mock.Anything).
			Return(fundingRecord, nil)

		// Setup target account credits query
		targetLiteIdentity := &protocol.LiteIdentity{
			CreditBalance: 10,
		}
		targetRecord := &api.AccountRecord{
			Account: targetLiteIdentity,
		}
		mockClient.On("Query", mock.Anything, mock.MatchedBy(func(u *url.URL) bool {
			return u.String() == targetAccount.URL.String()
		}), mock.Anything).
			Return(targetRecord, nil)

		// Capture the transaction to verify amount calculation
		var capturedTxn *protocol.Transaction
		envelope := &messaging.Envelope{}
		mockSigner.On("SignTransaction", mock.Anything, fundingAccount.URL, fundingAccount.Key.PrivateKey).
			Run(func(args mock.Arguments) {
				capturedTxn = args.Get(0).(*protocol.Transaction)
			}).
			Return(envelope, nil)

		// Setup transaction submission
		submissions := []*api.Submission{
			{
				Status: &protocol.TransactionStatus{
					Code: 0, // Success status
				},
			},
		}
		mockSubmitter.On("Submit", mock.Anything, envelope, mock.Anything).
			Return(submissions, nil)

		// Create credit manager and execute
		cm := NewCreditManager(mockClient, mockSubmitter, mockSigner, fundingAccount)
		err := cm.TopUpLiteAccount(context.Background(), targetAccount)

		// Verify the transaction
		require.NoError(t, err)
		require.NotNil(t, capturedTxn)
		
		addCredits, ok := capturedTxn.Body.(*protocol.AddCredits)
		require.True(t, ok)
		
		// Verify the amount calculation
		// CreditsToAdd (1000) * (AcmePrecision / CreditPrecision)
		expectedAmount := new(big.Int).SetUint64(CreditsToAdd)
		expectedAmount.Mul(expectedAmount, big.NewInt(protocol.AcmePrecision))
		expectedAmount.Div(expectedAmount, big.NewInt(protocol.CreditPrecision))
		
		assert.Equal(t, expectedAmount, &addCredits.Amount)
		assert.Equal(t, targetAccount.URL, addCredits.Recipient)
		assert.Equal(t, uint64(OraclePrice), addCredits.Oracle)
	})
}

// TestCreditManager_TransactionStatusHandling tests handling of transaction status
func TestCreditManager_TransactionStatusHandling(t *testing.T) {
	t.Run("handle failed transaction status", func(t *testing.T) {
		// Setup mocks
		mockClient := new(MockQueryClient)
		mockSubmitter := new(MockSubmitClient)
		mockSigner := new(MockTransactionSigner)

		// Create funding account
		fundingAccount := &LiteIdentity{
			URL:        mustParseURL("acc://lite-funding.acme/ACME"),
			Key:        &Key{PrivateKey: []byte("test-private-key")},
		}

		// Create target account
		targetAccount := &LiteIdentity{
			URL: mustParseURL("acc://lite-target.acme/ACME"),
		}

		// Setup queries
		fundingTokenAccount := &protocol.TokenAccount{
			Balance: *big.NewInt(1000 * protocol.AcmePrecision),
		}
		fundingRecord := &api.AccountRecord{
			Account: fundingTokenAccount,
		}
		mockClient.On("Query", mock.Anything, mock.MatchedBy(func(u *url.URL) bool {
			return u.String() == "acc://lite-funding.acme/ACME"
		}), mock.Anything).
			Return(fundingRecord, nil)

		targetLiteIdentity := &protocol.LiteIdentity{
			CreditBalance: 10,
		}
		targetRecord := &api.AccountRecord{
			Account: targetLiteIdentity,
		}
		mockClient.On("Query", mock.Anything, mock.MatchedBy(func(u *url.URL) bool {
			return u.String() == targetAccount.URL.String()
		}), mock.Anything).
			Return(targetRecord, nil)

		// Setup transaction signing
		envelope := &messaging.Envelope{}
		mockSigner.On("SignTransaction", mock.Anything, fundingAccount.URL, fundingAccount.Key.PrivateKey).
			Return(envelope, nil)

		// Setup transaction submission with failed status
		// Create a mock failed status 
		submissions := []*api.Submission{
			{
				Status: &protocol.TransactionStatus{
					Code: errors2.BadRequest, // Failed status
					Error: &errors2.Error{
						Message: "transaction failed",
					},
				},
			},
		}
		mockSubmitter.On("Submit", mock.Anything, envelope, mock.Anything).
			Return(submissions, nil)

		// Create credit manager and execute
		cm := NewCreditManager(mockClient, mockSubmitter, mockSigner, fundingAccount)
		err := cm.TopUpLiteAccount(context.Background(), targetAccount)

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "transaction failed")
	})

	t.Run("handle empty submissions", func(t *testing.T) {
		// Setup mocks
		mockClient := new(MockQueryClient)
		mockSubmitter := new(MockSubmitClient)
		mockSigner := new(MockTransactionSigner)

		// Create funding account
		fundingAccount := &LiteIdentity{
			URL:        mustParseURL("acc://lite-funding.acme/ACME"),
			Key:        &Key{PrivateKey: []byte("test-private-key")},
		}

		// Create target account
		targetAccount := &LiteIdentity{
			URL: mustParseURL("acc://lite-target.acme/ACME"),
		}

		// Setup queries
		fundingTokenAccount := &protocol.TokenAccount{
			Balance: *big.NewInt(1000 * protocol.AcmePrecision),
		}
		fundingRecord := &api.AccountRecord{
			Account: fundingTokenAccount,
		}
		mockClient.On("Query", mock.Anything, mock.MatchedBy(func(u *url.URL) bool {
			return u.String() == "acc://lite-funding.acme/ACME"
		}), mock.Anything).
			Return(fundingRecord, nil)

		targetLiteIdentity := &protocol.LiteIdentity{
			CreditBalance: 10,
		}
		targetRecord := &api.AccountRecord{
			Account: targetLiteIdentity,
		}
		mockClient.On("Query", mock.Anything, mock.MatchedBy(func(u *url.URL) bool {
			return u.String() == targetAccount.URL.String()
		}), mock.Anything).
			Return(targetRecord, nil)

		// Setup transaction signing
		envelope := &messaging.Envelope{}
		mockSigner.On("SignTransaction", mock.Anything, fundingAccount.URL, fundingAccount.Key.PrivateKey).
			Return(envelope, nil)

		// Setup transaction submission with empty result
		mockSubmitter.On("Submit", mock.Anything, envelope, mock.Anything).
			Return([]*api.Submission{}, nil)

		// Create credit manager and execute
		cm := NewCreditManager(mockClient, mockSubmitter, mockSigner, fundingAccount)
		err := cm.TopUpLiteAccount(context.Background(), targetAccount)

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no submissions returned")
	})
}