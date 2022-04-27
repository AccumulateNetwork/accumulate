package block_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	. "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() { acctesting.EnableDebugFeatures() }

func TestTransactionIsReady(tt *testing.T) {
	network := &config.Network{LocalSubnetID: tt.Name(), Type: config.BlockValidator}
	logger := NewTestLogger(tt)
	db := database.OpenInMemory(logger)
	t := NewBatchTest(tt, db)
	defer t.Discard()

	exec, err := block.NewNodeExecutor(block.ExecutorOptions{}, nil)
	require.NoError(t, err)

	// Foo's first authority and signer
	authority := new(FakeAuthority)
	authority.Url = url.MustParse("foo/authority")
	authority.AddAuthority(authority.Url)
	t.PutAccount(authority)

	signer := new(FakeSigner)
	signer.Url = authority.Url.JoinPath("signer")
	signer.Version = 1
	t.PutAccount(signer)

	// Foo's second authority and signer
	authority2 := new(FakeAuthority)
	authority2.Url = url.MustParse("foo/authority2")
	authority2.AddAuthority(authority2.Url)
	t.PutAccount(authority2)

	signer2 := new(FakeSigner)
	signer2.Url = authority2.Url.JoinPath("signer")
	signer2.Version = 1
	t.PutAccount(signer2)

	// An authority and signer that will not be an authority of foo
	unauthAuthority := new(FakeAuthority)
	unauthAuthority.Url = url.MustParse("foo/unauth-authority")
	unauthAuthority.AddAuthority(unauthAuthority.Url)
	t.PutAccount(unauthAuthority)

	unauthSigner := new(FakeSigner)
	unauthSigner.Url = unauthAuthority.Url.JoinPath("signer")
	unauthSigner.Version = 1
	t.PutAccount(unauthSigner)

	// An authority and signer belonging to a different root identity
	remoteAuthority := new(FakeAuthority)
	remoteAuthority.Url = url.MustParse("bar/authority")
	remoteAuthority.AddAuthority(remoteAuthority.Url)
	t.PutAccount(remoteAuthority)

	remoteSigner := new(FakeSigner)
	remoteSigner.Url = remoteAuthority.Url.JoinPath("signer")
	remoteSigner.Version = 1
	t.PutAccount(remoteSigner)

	// Foo's account
	account := new(FakeAccount)
	account.Url = url.MustParse("foo/account")
	account.AddAuthority(authority.Url)
	t.PutAccount(account)

	// The transaction
	body := new(FakeTransactionBody)
	body.TheType = protocol.TransactionTypeSendTokens
	txn := new(protocol.Transaction)
	txn.Header.Principal = account.Url
	txn.Body = body

	// The first signature
	sig := new(FakeSignature)
	sig.Signer = signer.Url
	sig.SignerVersion = signer.Version
	sig.Timestamp = 1
	sig.PublicKey = []byte{1}

	// Singlesig unsigned
	t.Run("Unsigned", func(t BatchTest) {
		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(network, t.Batch, txn, status)
		require.NoError(t, err)
		require.False(t, ready, "Expected the transaction to be not ready")
	})

	// Singlesig ready
	t.Run("Ready", func(t BatchTest) {
		t.AddSignature(txn.GetHash(), sig)

		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(network, t.Batch, txn, status)
		require.NoError(t, err)
		require.True(t, ready, "Expected the transaction to be ready")
	})

	// Multisig pending
	t.Run("Multisig Pending", func(t BatchTest) {
		signer := t.PutAccountCopy(signer).(*FakeSigner)
		signer.Threshold = 2

		t.AddSignature(txn.GetHash(), sig)

		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(network, t.Batch, txn, status)
		require.NoError(t, err)
		require.False(t, ready, "Expected the transaction to be not ready")
	})

	// Multisig ready
	t.Run("Multisig Ready", func(t BatchTest) {
		signer := t.PutAccountCopy(signer).(*FakeSigner)
		signer.Threshold = 2

		t.AddSignature(txn.GetHash(), sig)

		sig2 := sig.Copy()
		sig2.PublicKey = []byte{2}
		t.AddSignature(txn.GetHash(), sig2)

		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(network, t.Batch, txn, status)
		require.NoError(t, err)
		require.True(t, ready, "Expected the transaction to be ready")
	})

	// Multibook singlesig with one book ready
	t.Run("Multibook Pending", func(t BatchTest) {
		account := t.PutAccountCopy(account).(*FakeAccount)
		account.AddAuthority(authority2.Url)

		t.AddSignature(txn.GetHash(), sig)

		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(network, t.Batch, txn, status)
		require.NoError(t, err)
		require.False(t, ready, "Expected the transaction to be not ready")
	})

	// Multibook singlesig with all books ready
	t.Run("Multibook Ready", func(t BatchTest) {
		account := t.PutAccountCopy(account).(*FakeAccount)
		account.AddAuthority(authority2.Url)

		t.AddSignature(txn.GetHash(), sig)

		sig2 := sig.Copy()
		sig2.Signer = signer2.Url
		sig2.PublicKey = []byte{2}
		t.AddSignature(txn.GetHash(), sig2)

		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(network, t.Batch, txn, status)
		require.NoError(t, err)
		require.True(t, ready, "Expected the transaction to be ready")
	})

	// Disabled book with no signatures
	t.Run("Disabled Unsigned", func(t BatchTest) {
		account := t.PutAccountCopy(account).(*FakeAccount)
		entry, _ := account.GetAuthority(authority.Url)
		entry.Disabled = true

		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(network, t.Batch, txn, status)
		require.NoError(t, err)
		require.False(t, ready, "Expected the transaction to be not ready")
	})

	// Disabled book with an unauthorized signature
	t.Run("Disabled Single", func(t BatchTest) {
		account := t.PutAccountCopy(account).(*FakeAccount)
		entry, _ := account.GetAuthority(authority.Url)
		entry.Disabled = true

		sig := sig.Copy()
		sig.Signer = unauthSigner.Url
		t.AddSignature(txn.GetHash(), sig)

		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(network, t.Batch, txn, status)
		require.NoError(t, err)
		require.True(t, ready, "Expected the transaction to be ready")
	})

	// Multibook with one book disabled and an unauthorized signature
	t.Run("Disabled Pending", func(t BatchTest) {
		account := t.PutAccountCopy(account).(*FakeAccount)
		account.AddAuthority(authority2.Url)

		entry, _ := account.GetAuthority(authority.Url)
		entry.Disabled = true

		t.AddSignature(txn.GetHash(), sig)

		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(network, t.Batch, txn, status)
		require.NoError(t, err)
		require.False(t, ready, "Expected the transaction to be not ready")
	})

	// Multibook with one disabled book and an authorized signature for the
	// enabled book
	t.Run("Disabled Ready", func(t BatchTest) {
		account := t.PutAccountCopy(account).(*FakeAccount)
		account.AddAuthority(authority2.Url)

		entry, _ := account.GetAuthority(authority.Url)
		entry.Disabled = true

		sig1 := sig.Copy()
		sig1.Signer = url.MustParse("foo/non-authority/signer")
		t.AddSignature(txn.GetHash(), sig1)

		sig2 := sig.Copy()
		sig2.Signer = signer2.Url
		sig2.PublicKey = []byte{2}
		t.AddSignature(txn.GetHash(), sig2)

		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(network, t.Batch, txn, status)
		require.NoError(t, err)
		require.True(t, ready, "Expected the transaction to be ready")
	})

	// Multisig with an invalidated signature and a new signature
	t.Run("Invalidated", func(t BatchTest) {
		// This is not a unit test, because it's verifying AddSigner,
		// AddSignature, and TransactionIsReady network, in combination not in isolation.
		// But that's ok.

		signer := t.PutAccountCopy(signer).(*FakeSigner)
		signer.Threshold = 2

		// Signature @ version 1
		t.AddSignature(txn.GetHash(), sig)
		require.Equal(t, 1, t.GetSignatures(txn.GetHash(), signer.Url).Count())

		// Update the version
		signer.Version = 2

		// Signature @ version 2
		sig2 := sig.Copy()
		sig2.SignerVersion = 2
		sig2.PublicKey = []byte{2}
		t.AddSignature(txn.GetHash(), sig2)
		require.Equal(t, 1, t.GetSignatures(txn.GetHash(), signer.Url).Count())

		// Transaction is not ready
		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(network, t.Batch, txn, status)
		require.NoError(t, err)
		require.False(t, ready, "Expected the transaction to be not ready")
	})

	// Multibook multisig with old signatures that are still valid because they're the same version
	t.Run("Old Ready", func(t BatchTest) {
		account := t.PutAccountCopy(account).(*FakeAccount)
		account.AddAuthority(authority2.Url)

		signer := t.PutAccountCopy(signer).(*FakeSigner)
		signer.Threshold = 2
		signer2 := t.PutAccountCopy(signer2).(*FakeSigner)
		signer2.Threshold = 2

		// Two signatures for signer 1 @ version 1
		sig1_1 := sig.Copy()
		sig1_2 := sig.Copy()
		sig1_2.PublicKey = []byte{2}
		t.AddSignature(txn.GetHash(), sig1_1)
		t.AddSignature(txn.GetHash(), sig1_2)

		signer.Version++
		signer2.Version++

		// Two signatures for signer 1 @ version 1
		sig2_1 := sig.Copy()
		sig2_1.Signer = signer2.Url
		sig2_1.SignerVersion = 2
		sig2_2 := sig2_1.Copy()
		sig2_2.PublicKey = []byte{2}
		t.AddSignature(txn.GetHash(), sig2_1)
		t.AddSignature(txn.GetHash(), sig2_2)

		status := t.GetTxnStatus(txn.GetHash())
		ready, err := exec.TransactionIsReady(network, t.Batch, txn, status)
		require.NoError(t, err)
		require.True(t, ready, "Expected the transaction to be ready")
	})

}
