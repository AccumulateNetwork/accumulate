package blocks

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/cometbft"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// newTestGenesisDoc creates a genesis document with a single validator for testing.
func newTestGenesisDoc(t *testing.T, key ed25519.PublicKey) *cometbft.GenesisDoc {
	t.Helper()
	return &cometbft.GenesisDoc{
		Validators: []*cometbft.Validator{
			{
				PubKey: key,
				Power:  1,
			},
		},
	}
}

// newTestServer creates a mock HTTP server that serves a genesis document.
func newTestServer(t *testing.T, doc *cometbft.GenesisDoc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v3/genesis", r.URL.Path)
		b, err := json.Marshal(Response[*cometbft.GenesisDoc]{Value: doc})
		require.NoError(t, err)
		_, err = w.Write(b)
		require.NoError(t, err)
	}))
}

func TestValidateMajorBlock_Success(t *testing.T) {
	// Generate a key
	key := ed25519.NewKeyFromSeed(make([]byte, 32))

	// Set up a test server
	server := newTestServer(t, newTestGenesisDoc(t, key.Public().(ed25519.PublicKey)))
	defer server.Close()

	// Construct the block
	block := new(api.MajorBlockRecord)
	block.MinorBlocks = new(api.RecordRange[*api.MinorBlockRecord])
	block.MinorBlocks.Records = append(block.MinorBlocks.Records, new(api.MinorBlockRecord))
	block.MinorBlocks.Records[0].Entries = new(api.RecordRange[*api.ChainEntryRecord[api.Record]])

	// Add the anchor
	body := &protocol.BlockValidatorAnchor{PartitionAnchor: protocol.PartitionAnchor{MinorBlockIndex: 1}}
	txn := &protocol.Transaction{Body: body}
	msg := &messaging.TransactionMessage{Transaction: txn}
	anchorMsg := &messaging.BlockAnchor{Anchor: msg}
	block.MinorBlocks.Records[0].Entries.Records = append(block.MinorBlocks.Records[0].Entries.Records, &api.ChainEntryRecord[api.Record]{
		Value: &api.MessageRecord[messaging.Message]{Message: anchorMsg},
	})

	// Add the signature
	hash := anchorMsg.Hash()
	sig := &protocol.ED25519Signature{
		PublicKey: key.Public().(ed25519.PublicKey),
		Signature: ed25519.Sign(key, hash[:]),
	}
	block.MinorBlocks.Records[0].Entries.Records = append(block.MinorBlocks.Records[0].Entries.Records, &api.ChainEntryRecord[api.Record]{
		Value: &api.SignatureSetRecord{
			Signatures: &api.RecordRange[*api.MessageRecord[messaging.Message]]{
				Records: []*api.MessageRecord[messaging.Message]{{
					Message: &messaging.SignatureMessage{Signature: sig},
				}},
			},
		},
	})

	// Create the validator
	v := NewBlockValidator(NewGenesisAuthorityProvider(http.DefaultClient, server.URL))

	// Validate
	err := ValidateMajorBlock(context.Background(), block, v)
	require.NoError(t, err)
}

func TestValidateMajorBlock_NoAnchor(t *testing.T) {
	// Set up a test server
	server := newTestServer(t, new(cometbft.GenesisDoc))
	defer server.Close()

	// Construct the block
	block := new(api.MajorBlockRecord)
	block.MinorBlocks = new(api.RecordRange[*api.MinorBlockRecord])
	block.MinorBlocks.Records = append(block.MinorBlocks.Records, new(api.MinorBlockRecord))
	block.MinorBlocks.Records[0].Entries = new(api.RecordRange[*api.ChainEntryRecord[api.Record]])

	// Create the validator
	v := NewBlockValidator(NewGenesisAuthorityProvider(http.DefaultClient, server.URL))

	// Validate
	err := ValidateMajorBlock(context.Background(), block, v)
	require.ErrorContains(t, err, "missing its anchor")
}

func TestValidateMajorBlock_ValidatorError(t *testing.T) {
	// Set up a test server with no authorities
	server := newTestServer(t, new(cometbft.GenesisDoc))

	// Construct the block
	block := new(api.MajorBlockRecord)
	block.MinorBlocks = new(api.RecordRange[*api.MinorBlockRecord])
	block.MinorBlocks.Records = append(block.MinorBlocks.Records, new(api.MinorBlockRecord))
	block.MinorBlocks.Records[0].Entries = new(api.RecordRange[*api.ChainEntryRecord[api.Record]])

	// Add the anchor
	body := &protocol.BlockValidatorAnchor{PartitionAnchor: protocol.PartitionAnchor{MinorBlockIndex: 1}}
	txn := &protocol.Transaction{Body: body}
	msg := &messaging.TransactionMessage{Transaction: txn}
	anchorMsg := &messaging.BlockAnchor{Anchor: msg}
	block.MinorBlocks.Records[0].Entries.Records = append(block.MinorBlocks.Records[0].Entries.Records, &api.ChainEntryRecord[api.Record]{
		Value: &api.MessageRecord[messaging.Message]{Message: anchorMsg},
	})

	// Create the validator
	v := NewBlockValidator(NewGenesisAuthorityProvider(http.DefaultClient, server.URL))

	// Close the server to cause an error
	server.Close()

	// Validate
	err := ValidateMajorBlock(context.Background(), block, v)
	require.Error(t, err)
}

func TestValidateMajorBlock_BadSignature(t *testing.T) {
	// Generate a key
	key := ed25519.NewKeyFromSeed(make([]byte, 32))

	// Set up a test server
	server := newTestServer(t, newTestGenesisDoc(t, key.Public().(ed25519.PublicKey)))
	defer server.Close()

	// Construct the block
	block := new(api.MajorBlockRecord)
	block.MinorBlocks = new(api.RecordRange[*api.MinorBlockRecord])
	block.MinorBlocks.Records = append(block.MinorBlocks.Records, new(api.MinorBlockRecord))
	block.MinorBlocks.Records[0].Entries = new(api.RecordRange[*api.ChainEntryRecord[api.Record]])

	// Add the anchor
	body := &protocol.BlockValidatorAnchor{PartitionAnchor: protocol.PartitionAnchor{MinorBlockIndex: 1}}
	txn := &protocol.Transaction{Body: body}
	msg := &messaging.TransactionMessage{Transaction: txn}
	anchorMsg := &messaging.BlockAnchor{Anchor: msg}
	block.MinorBlocks.Records[0].Entries.Records = append(block.MinorBlocks.Records[0].Entries.Records, &api.ChainEntryRecord[api.Record]{
		Value: &api.MessageRecord[messaging.Message]{Message: anchorMsg},
	})

	// Add the signature
	hash := anchorMsg.Hash()
	sig := &protocol.ED25519Signature{
		PublicKey: key.Public().(ed25519.PublicKey),
		Signature: ed25519.Sign(key, hash[:]),
	}
	block.MinorBlocks.Records[0].Entries.Records = append(block.MinorBlocks.Records[0].Entries.Records, &api.ChainEntryRecord[api.Record]{
		Value: &api.SignatureSetRecord{
			Signatures: &api.RecordRange[*api.MessageRecord[messaging.Message]]{
				Records: []*api.MessageRecord[messaging.Message]{{
					Message: &messaging.SignatureMessage{Signature: sig},
				}},
			},
		},
	})

	// Create the validator
	v := NewBlockValidator(NewGenesisAuthorityProvider(http.DefaultClient, server.URL))

	// Validate
	err := ValidateMajorBlock(context.Background(), block, v)
	require.ErrorContains(t, err, "not enough signatures: got 0, want 1")
}

func TestValidateMajorBlock_UnauthorizedSigner(t *testing.T) {
	// Generate keys
	signerKey := ed25519.NewKeyFromSeed(make([]byte, 32))
	authorityKey := ed25519.NewKeyFromSeed([]byte("real authority key has 32 bytes"))

	// Set up a test server
	server := newTestServer(t, newTestGenesisDoc(t, authorityKey.Public().(ed25519.PublicKey)))
	defer server.Close()

	// Construct the block
	block := new(api.MajorBlockRecord)
	block.MinorBlocks = new(api.RecordRange[*api.MinorBlockRecord])
	block.MinorBlocks.Records = append(block.MinorBlocks.Records, new(api.MinorBlockRecord))
	block.MinorBlocks.Records[0].Entries = new(api.RecordRange[*api.ChainEntryRecord[api.Record]])

	// Add the anchor
	body := &protocol.BlockValidatorAnchor{PartitionAnchor: protocol.PartitionAnchor{MinorBlockIndex: 1}}
	txn := &protocol.Transaction{Body: body}
	msg := &messaging.TransactionMessage{Transaction: txn}
	anchorMsg := &messaging.BlockAnchor{Anchor: msg}
	block.MinorBlocks.Records[0].Entries.Records = append(block.MinorBlocks.Records[0].Entries.Records, &api.ChainEntryRecord[api.Record]{
		Value: &api.MessageRecord[messaging.Message]{Message: anchorMsg},
	})

	// Add the signature from the signer
	hash := anchorMsg.Hash()
	sig := &protocol.ED25519Signature{
		PublicKey: signerKey.Public().(ed25519.PublicKey),
		Signature: ed25519.Sign(signerKey, hash[:]),
	}
	block.MinorBlocks.Records[0].Entries.Records = append(block.MinorBlocks.Records[0].Entries.Records, &api.ChainEntryRecord[api.Record]{
		Value: &api.SignatureSetRecord{
			Signatures: &api.RecordRange[*api.MessageRecord[messaging.Message]]{
				Records: []*api.MessageRecord[messaging.Message]{{
					Message: &messaging.SignatureMessage{Signature: sig},
				}},
			},
		},
	})

	// Create the validator
	v := NewBlockValidator(NewGenesisAuthorityProvider(http.DefaultClient, server.URL))

	// Validate
	err := ValidateMajorBlock(context.Background(), block, v)
	require.ErrorContains(t, err, "not enough signatures: got 0, want 1")
}

func TestValidateMajorBlock_ThresholdNotMet(t *testing.T) {
	// Generate a key
	key := ed25519.NewKeyFromSeed(make([]byte, 32))

	// Set up a test server with a validator that has a power of 2, requiring 2 votes
	// The threshold is floor(2/3*P)+1. If P=1, T=1. If P=2, T=2. If P=3, T=3.
	// So to get a threshold of 2, we need at least 2 total power.
	doc := newTestGenesisDoc(t, key.Public().(ed25519.PublicKey))
	doc.Validators = append(doc.Validators, &cometbft.Validator{
		PubKey: ed25519.NewKeyFromSeed(make([]byte, 33)).Public().(ed25519.PublicKey),
		Power:  1,
	})
	server := newTestServer(t, doc)
	defer server.Close()

	// Construct the block
	block := new(api.MajorBlockRecord)
	block.MinorBlocks = new(api.RecordRange[*api.MinorBlockRecord])
	block.MinorBlocks.Records = append(block.MinorBlocks.Records, new(api.MinorBlockRecord))
	block.MinorBlocks.Records[0].Entries = new(api.RecordRange[*api.ChainEntryRecord[api.Record]])

	// Add the anchor
	body := &protocol.BlockValidatorAnchor{PartitionAnchor: protocol.PartitionAnchor{MinorBlockIndex: 1}}
	txn := &protocol.Transaction{Body: body}
	msg := &messaging.TransactionMessage{Transaction: txn}
	anchorMsg := &messaging.BlockAnchor{Anchor: msg}
	block.MinorBlocks.Records[0].Entries.Records = append(block.MinorBlocks.Records[0].Entries.Records, &api.ChainEntryRecord[api.Record]{
		Value: &api.MessageRecord[messaging.Message]{Message: anchorMsg},
	})

	// Add the signature
	hash := anchorMsg.Hash()
	sig := &protocol.ED25519Signature{
		PublicKey: key.Public().(ed25519.PublicKey),
		Signature: ed25519.Sign(key, hash[:]),
	}
	block.MinorBlocks.Records[0].Entries.Records = append(block.MinorBlocks.Records[0].Entries.Records, &api.ChainEntryRecord[api.Record]{
		Value: &api.SignatureSetRecord{
			Signatures: &api.RecordRange[*api.MessageRecord[messaging.Message]]{
				Records: []*api.MessageRecord[messaging.Message]{{
					Message: &messaging.SignatureMessage{Signature: sig},
				}},
			},
		},
	})

	// Create the validator
	v := NewBlockValidator(NewGenesisAuthorityProvider(http.DefaultClient, server.URL))

	// Validate
	err := ValidateMajorBlock(context.Background(), block, v)
	require.ErrorContains(t, err, "not enough signatures: got 1, want 2")
}

func TestBlockValidator_MissingTxHash(t *testing.T) {
	// Create the validator
	v := new(BlockValidator)

	// Validate
	_, err := v.Validate(context.Background(), new(messaging.Envelope), api.ValidateOptions{})
	require.ErrorContains(t, err, "missing transaction hash")
}

// mockValidator is a mock implementation of api.Validator for testing.
type mockValidator struct {
	err error
}

// Validate implements api.Validator.
func (v *mockValidator) Validate(context.Context, *messaging.Envelope, api.ValidateOptions) ([]*api.Submission, error) {
	return nil, v.err
}

func TestValidateMessageRecord(t *testing.T) {
	// Create a dummy validator that always succeeds
	v := &mockValidator{err: nil}

	// A nil record is not an error
	ok, err := ValidateMessageRecord(context.Background(), v, nil)
	require.NoError(t, err)
	require.False(t, ok)

	// A record with a nil message is not an error
	ok, err = ValidateMessageRecord(context.Background(), v, new(api.MessageRecord[messaging.Message]))
	require.NoError(t, err)
	require.False(t, ok)

	// A valid record is valid
	rec := &api.MessageRecord[messaging.Message]{Message: new(messaging.TransactionMessage)}
	ok, err = ValidateMessageRecord(context.Background(), v, rec)
	require.NoError(t, err)
	require.True(t, ok)
}
