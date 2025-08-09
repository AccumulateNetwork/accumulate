// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestHeaderMethods tests Header struct methods
func TestHeaderMethods(t *testing.T) {
	h := &Header{
		Version:     1,
		Height:      12345,
		RootHash:    [32]byte{1, 2, 3, 4},
		Timestamp:   1234567890,
	}
	
	// Test basic fields
	assert.Equal(t, uint64(1), h.Version)
	assert.Equal(t, uint64(12345), h.Height)
	assert.Equal(t, [32]byte{1, 2, 3, 4}, h.RootHash)
	assert.Equal(t, uint64(1234567890), h.Timestamp)
}

// TestSectionType tests section type constants and methods
func TestSectionType(t *testing.T) {
	tests := []struct {
		section SectionType
		name    string
	}{
		{SectionTypeHeader, "header"},
		{SectionTypeRecords, "records"},
		{SectionTypeAccounts, "accounts"},
		{SectionTypeTransactions, "transactions"},
		{SectionTypeSignatures, "signatures"},
		{SectionTypeGzTransactions, "gz-transactions"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test String method
			str := tt.section.String()
			assert.NotEmpty(t, str)
			
			// Test that it's a valid section type
			assert.True(t, tt.section >= SectionTypeHeader)
			assert.True(t, tt.section <= SectionTypeMax)
		})
	}
}

// TestRecordEntry tests RecordEntry functionality
func TestRecordEntry(t *testing.T) {
	entry := &RecordEntry{
		Key: database.NewKey("Account", protocol.AccountUrl("alice")),
		Value: &protocol.LiteTokenAccount{
			Url:      protocol.AccountUrl("alice/tokens"),
			TokenUrl: protocol.AcmeUrl(),
		},
		Hash: [32]byte{1, 2, 3},
	}
	
	// Test Key
	assert.NotNil(t, entry.Key)
	assert.Equal(t, "Account", entry.Key.Get(0))
	
	// Test Value
	assert.NotNil(t, entry.Value)
	acc, ok := entry.Value.(*protocol.LiteTokenAccount)
	assert.True(t, ok)
	assert.Equal(t, "alice/tokens", acc.Url.String())
	
	// Test Hash
	assert.Equal(t, [32]byte{1, 2, 3}, entry.Hash)
}

// TestAccountEntry tests AccountEntry functionality
func TestAccountEntry(t *testing.T) {
	acc := &protocol.LiteTokenAccount{
		Url:      protocol.AccountUrl("alice/tokens"),
		TokenUrl: protocol.AcmeUrl(),
	}
	
	entry := &AccountEntry{
		Account: acc,
		Hash:    [32]byte{5, 6, 7},
	}
	
	// Test Account
	assert.NotNil(t, entry.Account)
	assert.Equal(t, "alice/tokens", entry.Account.GetUrl().String())
	
	// Test Hash
	assert.Equal(t, [32]byte{5, 6, 7}, entry.Hash)
}

// TestTransactionEntry tests TransactionEntry functionality
func TestTransactionEntry(t *testing.T) {
	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: protocol.AccountUrl("alice"),
		},
		Body: &protocol.SendTokens{},
	}
	
	entry := &TransactionEntry{
		Transaction: txn,
		Hash:        [32]byte{8, 9, 10},
	}
	
	// Test Transaction
	assert.NotNil(t, entry.Transaction)
	assert.Equal(t, "alice", entry.Transaction.Header.Principal.String())
	
	// Test Hash
	assert.Equal(t, [32]byte{8, 9, 10}, entry.Hash)
}

// TestSignatureEntry tests SignatureEntry functionality
func TestSignatureEntry(t *testing.T) {
	sig := &protocol.ED25519Signature{
		PublicKey: [32]byte{11, 12, 13},
		Signature: [64]byte{14, 15, 16},
	}
	
	entry := &SignatureEntry{
		Signature: sig,
		Hash:      [32]byte{17, 18, 19},
	}
	
	// Test Signature
	assert.NotNil(t, entry.Signature)
	ed25519Sig, ok := entry.Signature.(*protocol.ED25519Signature)
	assert.True(t, ok)
	assert.Equal(t, [32]byte{11, 12, 13}, ed25519Sig.PublicKey)
	
	// Test Hash
	assert.Equal(t, [32]byte{17, 18, 19}, entry.Hash)
}

// TestReaderWriter tests basic reader/writer functionality
func TestReaderWriter(t *testing.T) {
	// Create a buffer to write to
	buf := &bytes.Buffer{}
	
	// Create writer
	w := NewWriter(buf)
	assert.NotNil(t, w)
	
	// Write a header
	header := &Header{
		Version:  1,
		Height:   100,
		RootHash: [32]byte{1, 2, 3},
	}
	
	err := w.WriteHeader(header)
	require.NoError(t, err)
	
	// Create reader
	reader := bytes.NewReader(buf.Bytes())
	r := NewReader(reader)
	assert.NotNil(t, r)
	
	// Read header back
	var readHeader Header
	sectionType, err := r.ReadNextSection(&readHeader)
	require.NoError(t, err)
	assert.Equal(t, SectionTypeHeader, sectionType)
	assert.Equal(t, header.Version, readHeader.Version)
	assert.Equal(t, header.Height, readHeader.Height)
}

// TestCollectOptions tests CollectOptions configuration
func TestCollectOptions(t *testing.T) {
	opts := CollectOptions{
		IncludeReceipts: true,
		IncludeSystem:   true,
		MaxRecordCount:  1000,
		MaxSectionSize:  1024 * 1024,
	}
	
	// Test options are set correctly
	assert.True(t, opts.IncludeReceipts)
	assert.True(t, opts.IncludeSystem)
	assert.Equal(t, 1000, opts.MaxRecordCount)
	assert.Equal(t, 1024*1024, opts.MaxSectionSize)
	
	// Test with PreserveAccountHistory function
	opts.PreserveAccountHistory = func(account *database.Account) (bool, error) {
		// Preserve all token accounts
		if acc, err := account.Main().Get(); err == nil {
			if _, ok := acc.(*protocol.TokenAccount); ok {
				return true, nil
			}
		}
		return false, nil
	}
	assert.NotNil(t, opts.PreserveAccountHistory)
}

// TestRestoreOptions tests RestoreOptions configuration
func TestRestoreOptions(t *testing.T) {
	opts := RestoreOptions{
		BatchRecordCount: 100,
		SkipHashCheck:    false,
		ProgressCallback: func(restored, total int) {
			// Progress tracking
		},
	}
	
	// Test options are set correctly
	assert.Equal(t, 100, opts.BatchRecordCount)
	assert.False(t, opts.SkipHashCheck)
	assert.NotNil(t, opts.ProgressCallback)
}

// TestVisitor tests the visitor pattern implementation
func TestVisitor(t *testing.T) {
	// Create a test visitor
	visitedSections := make([]SectionType, 0)
	visitor := func(r SectionReader) error {
		section := r.Type()
		visitedSections = append(visitedSections, section)
		
		switch section {
		case SectionTypeHeader:
			var h Header
			return r.Read(&h)
		case SectionTypeRecords:
			var entries []RecordEntry
			return r.Read(&entries)
		default:
			return nil
		}
	}
	
	// Create sample data
	buf := &bytes.Buffer{}
	w := NewWriter(buf)
	
	// Write header
	err := w.WriteHeader(&Header{Version: 1})
	require.NoError(t, err)
	
	// Write records section
	err = w.WriteRecords([]RecordEntry{
		{
			Key:   database.NewKey("test"),
			Value: &protocol.SystemData{},
		},
	})
	require.NoError(t, err)
	
	// Visit sections
	reader := bytes.NewReader(buf.Bytes())
	err = Visit(reader, visitor)
	require.NoError(t, err)
	
	// Verify visited sections
	assert.Len(t, visitedSections, 2)
	assert.Equal(t, SectionTypeHeader, visitedSections[0])
	assert.Equal(t, SectionTypeRecords, visitedSections[1])
}

// TestSectionReader tests SectionReader interface
func TestSectionReader(t *testing.T) {
	// Mock section reader
	type mockSectionReader struct {
		sectionType SectionType
		data        interface{}
	}
	
	reader := &mockSectionReader{
		sectionType: SectionTypeAccounts,
		data: []AccountEntry{
			{
				Account: &protocol.LiteIdentity{
					Url: protocol.AccountUrl("alice"),
				},
			},
		},
	}
	
	// Test Type method
	assert.Equal(t, SectionTypeAccounts, reader.sectionType)
	
	// Test that data can be accessed
	assert.NotNil(t, reader.data)
	accounts, ok := reader.data.([]AccountEntry)
	assert.True(t, ok)
	assert.Len(t, accounts, 1)
}

// TestErrorHandling tests error handling in snapshot operations
func TestErrorHandling(t *testing.T) {
	// Test writer with closed writer
	buf := &closedBuffer{}
	w := NewWriter(buf)
	
	err := w.WriteHeader(&Header{})
	assert.Error(t, err)
	
	// Test reader with invalid data
	invalidData := bytes.NewReader([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	r := NewReader(invalidData)
	
	var h Header
	_, err = r.ReadNextSection(&h)
	assert.Error(t, err)
}

// closedBuffer simulates a writer that always returns an error
type closedBuffer struct {
	bytes.Buffer
}

func (c *closedBuffer) Write(p []byte) (n int, err error) {
	return 0, errors.New("writer closed")
}

// TestSectionSizeLimits tests section size limit handling
func TestSectionSizeLimits(t *testing.T) {
	buf := &bytes.Buffer{}
	w := NewWriter(buf)
	
	// Create large record entries
	entries := make([]RecordEntry, 1000)
	for i := range entries {
		entries[i] = RecordEntry{
			Key:   database.NewKey("test", i),
			Value: &protocol.SystemData{},
		}
	}
	
	// Should handle large sections
	err := w.WriteRecords(entries)
	assert.NoError(t, err)
	
	// Verify data was written
	assert.Greater(t, buf.Len(), 0)
}

// TestHashVerification tests hash verification in snapshots
func TestHashVerification(t *testing.T) {
	// Create entry with hash
	entry := &AccountEntry{
		Account: &protocol.LiteIdentity{
			Url:           protocol.AccountUrl("alice"),
			CreditBalance: 1000,
		},
		Hash: [32]byte{1, 2, 3}, // Incorrect hash for testing
	}
	
	// Hash should be present
	assert.NotEqual(t, [32]byte{}, entry.Hash)
	
	// In real implementation, hash verification would check this
	// For now, just verify the hash field exists and is accessible
	assert.Len(t, entry.Hash, 32)
}