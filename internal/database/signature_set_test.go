package database_test

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() { acctesting.EnableDebugFeatures() }

func TestSignatureSet_Add(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	signer := new(acctesting.FakeSigner)
	signer.Url = url.MustParse("signer")
	signer.Version = 1
	require.NoError(t, batch.Account(signer.Url).PutState(signer))

	hash := sha256.Sum256([]byte(t.Name()))
	key := acctesting.GenerateKey(t.Name())
	builder := &signing.Builder{
		InitMode:  signing.InitWithSimpleHash,
		Url:       signer.Url,
		Signer:    signing.PrivateKey(key),
		Version:   signer.Version,
		Timestamp: 1,
	}
	edSig, err := builder.SetType(protocol.SignatureTypeED25519).Sign(hash[:])
	require.NoError(t, err)
	rcdSig, err := builder.SetType(protocol.SignatureTypeRCD1).Sign(hash[:])
	require.NoError(t, err)

	count, err := batch.Transaction(hash[:]).AddSignature(edSig)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	count, err = batch.Transaction(hash[:]).AddSignature(rcdSig)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}
