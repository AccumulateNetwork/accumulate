package database_test

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	. "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestVersionedSignatureSet_WrongSigner(t *testing.T) {
	db := database.OpenInMemory(nil)
	cs := db.Begin(true)

	// Foo's first authority and signer
	authority := new(FakeAuthority)
	authority.Url = protocol.AccountUrl("foo", "authority")
	authority.AddAuthority(authority.Url)
	require.NoError(t, cs.Account(authority.Url).State().Put(authority))

	signerKey := GenerateKey("foo")
	signerKeyHash := sha256.Sum256(signerKey[32:])

	signer := new(FakeSigner)
	signer.Url = authority.Url.JoinPath("signer")
	signer.Version = 1
	signer.Keys = append(signer.Keys, &protocol.KeySpec{PublicKeyHash: signerKeyHash[:]})
	require.NoError(t, cs.Account(signer.Url).State().Put(signer))

	// Foo's account
	account := new(FakeAccount)
	account.Url = protocol.AccountUrl("foo", "account")
	account.AddAuthority(authority.Url)
	require.NoError(t, cs.Account(account.Url).State().Put(account))

	// The transaction
	body := new(FakeTransactionBody)
	body.TheType = protocol.TransactionTypeSendTokens
	txn := new(protocol.Transaction)
	txn.Header.Principal = account.Url
	txn.Body = body

	// The signature
	sig := new(FakeSignature)
	sig.Signer = protocol.AccountUrl("bar", "authority", "signer")
	sig.SignerVersion = signer.Version
	sig.Timestamp = 1
	sig.PublicKey = signerKey[32:]

	// Add the signature
	require.Error(t, cs.Transaction(txn.GetHash()).Signatures(signer.Url).Add(sig))
}

func TestVersionedSignatureSet_AddSigner(t *testing.T) {
	db := database.OpenInMemory(nil)
	cs := db.Begin(true)

	// Foo's first authority and signer
	authority := new(FakeAuthority)
	authority.Url = protocol.AccountUrl("foo", "authority")
	authority.AddAuthority(authority.Url)
	require.NoError(t, cs.Account(authority.Url).State().Put(authority))

	signerKey := GenerateKey("foo")
	signerKeyHash := sha256.Sum256(signerKey[32:])

	signer := new(FakeSigner)
	signer.Url = authority.Url.JoinPath("signer")
	signer.Version = 1
	signer.Keys = append(signer.Keys, &protocol.KeySpec{PublicKeyHash: signerKeyHash[:]})
	require.NoError(t, cs.Account(signer.Url).State().Put(signer))

	// Foo's account
	account := new(FakeAccount)
	account.Url = protocol.AccountUrl("foo", "account")
	account.AddAuthority(authority.Url)
	require.NoError(t, cs.Account(account.Url).State().Put(account))

	// The transaction
	body := new(FakeTransactionBody)
	body.TheType = protocol.TransactionTypeSendTokens
	txn := new(protocol.Transaction)
	txn.Header.Principal = account.Url
	txn.Body = body

	// The signature
	sig := new(FakeSignature)
	sig.Signer = signer.Url
	sig.SignerVersion = signer.Version
	sig.Timestamp = 1
	sig.PublicKey = signerKey[32:]

	// Add the signature
	require.NoError(t, cs.Transaction(txn.GetHash()).Signatures(signer.Url).Add(sig))

	// Verify the signer was added
	_, err := cs.Transaction(txn.GetHash()).Signers().Index(signer.Url)
	require.NoError(t, err)
	status, err := cs.Transaction(txn.GetHash()).Status().Get()
	require.NoError(t, err)
	_, ok := status.GetSigner(signer.Url)
	require.True(t, ok)
}

func TestVersionedSignatureSet_LiteToken(t *testing.T) {
	db := database.OpenInMemory(nil)
	cs := db.Begin(true)

	// Lite token account
	liteKey := GenerateKey(t.Name())
	liteUrl := AcmeLiteAddressStdPriv(liteKey)
	liteId := new(protocol.LiteIdentity)
	liteId.Url = liteUrl.RootIdentity()
	liteAcct := new(protocol.LiteTokenAccount)
	liteAcct.Url = liteUrl
	require.NoError(t, cs.Account(liteId.Url).State().Put(liteId))
	require.NoError(t, cs.Account(liteAcct.Url).State().Put(liteAcct))

	// The transaction
	body := new(FakeTransactionBody)
	body.TheType = protocol.TransactionTypeSendTokens
	txn := new(protocol.Transaction)
	txn.Header.Principal = liteUrl
	txn.Body = body

	// The signature
	sig := new(FakeSignature)
	sig.Signer = liteUrl
	sig.SignerVersion = 1
	sig.Timestamp = 1
	sig.PublicKey = liteKey[32:]

	// Add the signature
	require.NoError(t, cs.Transaction(txn.GetHash()).Signatures(liteUrl).Add(sig))

	// Verify the signer was added
	_, err := cs.Transaction(txn.GetHash()).Signers().Index(liteId.Url)
	require.NoError(t, err)
	status, err := cs.Transaction(txn.GetHash()).Status().Get()
	require.NoError(t, err)
	_, ok := status.GetSigner(liteId.Url)
	require.True(t, ok)
}
