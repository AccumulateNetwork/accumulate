package blocks

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestPhase2_EndToEndValidation(t *testing.T) {
	// 1. Setup: Create mock authorities and keys
	_, signers, pubKeys := newMockAuthorities(t, 4)
	threshold := 3 // 3 out of 4 must sign

	// 2. Build the message and sign it
	anchor := &messaging.BlockAnchor{
		Anchor: &messaging.TransactionMessage{Transaction: &protocol.Transaction{Body: &protocol.WriteData{}}},
	}
	hash := anchor.Hash()

	// Have a quorum of authorities sign the hash
	signatures := make([]protocol.Signature, threshold)
	for i := 0; i < threshold; i++ {
		sigBytes, err := signers[i].Sign(hash[:])
		require.NoError(t, err)
		sig := new(protocol.ED25519Signature)
		require.NoError(t, sig.UnmarshalBinary(sigBytes))
		signatures[i] = sig
	}

	// 3. Construct the Major Block Record
	majorBlock := &api.MajorBlockRecord{
		Index: 1,
		Time:  time.Now(),
		MinorBlocks: &api.RecordRange[*api.MinorBlockRecord]{
			Records: []*api.MinorBlockRecord{
				{
					Entries: &api.RecordRange[*api.ChainEntryRecord[api.Record]]{
						Records: []*api.ChainEntryRecord[api.Record]{
							{Value: &api.MessageRecord[messaging.Message]{Message: anchor}},
							{Value: &api.SignatureSetRecord{
								Signatures: convertSigsToRecords(wrapSignatures(signatures)),
							}},
						},
					},
				},
			},
		},
	}

	// 4. Configure the validator
	authMap := make(map[[32]byte]bool, len(pubKeys))
	for _, pubKey := range pubKeys {
		keyHash := sha256.Sum256(pubKey)
		authMap[keyHash] = true
	}
	authProvider := &StaticAuthorityProvider{
		Authorities: authMap,
		Threshold:   uint64(threshold),
	}
	validator := &BlockValidator{
		Authorities: authProvider,
	}

	// --- Run Tests ---
	t.Run("Success", func(t *testing.T) {
		err := ValidateMajorBlock(context.Background(), majorBlock, validator)
		require.NoError(t, err)
	})

	t.Run("Failure - Not enough signatures", func(t *testing.T) {
		// Create a block with only 2 signatures, which is below the threshold of 3
		insufficientSigsBlock := shallowCopyMajorBlock(majorBlock)
		insufficientSigsBlock.MinorBlocks.Records[0].Entries.Records[1].Value.(*api.SignatureSetRecord).Signatures = convertSigsToRecords(wrapSignatures(signatures[:threshold-1]))

		err := ValidateMajorBlock(context.Background(), insufficientSigsBlock, validator)
		require.Error(t, err)
		require.ErrorContains(t, err, "not enough valid signatures")
	})

	t.Run("Failure - No anchor", func(t *testing.T) {
		// Create a block with signatures but no anchor message
		noAnchorBlock := shallowCopyMajorBlock(majorBlock)
		noAnchorBlock.MinorBlocks.Records[0].Entries.Records[0].Value = &api.MessageRecord[messaging.Message]{Message: &messaging.TransactionMessage{}}

		err := ValidateMajorBlock(context.Background(), noAnchorBlock, validator)
		require.Error(t, err)
		require.ErrorContains(t, err, "missing its anchor")
	})
}

// newMockAuthorities creates a set of mock authorities and corresponding signers.
func newMockAuthorities(t *testing.T, count int) ([]protocol.Authority, []build.Signer, []ed25519.PublicKey) {
	t.Helper()
	authorities := make([]protocol.Authority, count)
	signers := make([]build.Signer, count)
	pubKeys := make([]ed25519.PublicKey, count)

	for i := range authorities {
		pubKey, privKey, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		// Use a lite identity for the mock authority
		url, err := protocol.LiteTokenAddress(pubKey, "ACME", protocol.SignatureTypeED25519)
		require.NoError(t, err)

		authorities[i] = &protocol.LiteIdentity{Url: url}
		signers[i] = build.ED25519PrivateKey(privKey)
		pubKeys[i] = pubKey
	}
	return authorities, signers, pubKeys
}

// wrapSignatures converts a slice of signatures into the structure needed for the API record.
func wrapSignatures(sigs []protocol.Signature) []*api.MessageRecord[messaging.MessageWithSignature] {
	wrapped := make([]*api.MessageRecord[messaging.MessageWithSignature], len(sigs))
	for i, sig := range sigs {
		wrapped[i] = &api.MessageRecord[messaging.MessageWithSignature]{
			Message: &messaging.SignatureMessage{
				Signature: sig,
			},
		}
	}
	return wrapped
}

// shallowCopyMajorBlock creates a shallow copy of a major block record to avoid modifying the original in sub-tests.
func convertSigsToRecords(sigs []*api.MessageRecord[messaging.MessageWithSignature]) *api.RecordRange[*api.MessageRecord[messaging.Message]] {
	records := make([]*api.MessageRecord[messaging.Message], len(sigs))
	for i, sig := range sigs {
		records[i] = &api.MessageRecord[messaging.Message]{Message: sig.Message}
	}
	return &api.RecordRange[*api.MessageRecord[messaging.Message]]{Records: records}
}

// shallowCopyMajorBlock creates a shallow copy of a major block record to avoid modifying the original in sub-tests.
func shallowCopyMajorBlock(b *api.MajorBlockRecord) *api.MajorBlockRecord {
	copy := *b
	copy.MinorBlocks = &api.RecordRange[*api.MinorBlockRecord]{
		Records: make([]*api.MinorBlockRecord, len(b.MinorBlocks.Records)),
	}
	for i, minor := range b.MinorBlocks.Records {
		minorCopy := *minor
		minorCopy.Entries = &api.RecordRange[*api.ChainEntryRecord[api.Record]]{
			Records: make([]*api.ChainEntryRecord[api.Record], len(minor.Entries.Records)),
		}
		copy.MinorBlocks.Records[i] = &minorCopy
		for j, entry := range minor.Entries.Records {
			minorCopy.Entries.Records[j] = &api.ChainEntryRecord[api.Record]{Value: entry.Value}
		}
	}
	return &copy
}
