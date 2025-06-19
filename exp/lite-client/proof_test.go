package liteclient

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	dbMerkle "gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
)

type mockLiteClient struct {
	*LiteClient
	mockProof *dbMerkle.Receipt
	mockRoot  []byte
}

func (m *mockLiteClient) FetchProof(ctx context.Context, account string) (*VerifiedAccount, error) {
	return &VerifiedAccount{
		Url:     account,
		Receipt: m.mockProof,
		Height:  12345,
	}, nil
}

func TestFetchProofAndVerifyProof(t *testing.T) {
	// 1. Construct a trivial Merkle receipt: hash0 -> hash1
	fmt.Println("[Step 1] Constructing Merkle receipt...")
	hash0 := sha256.Sum256([]byte("leaf"))
	fmt.Printf("  hash0 (leaf hash): %x\n", hash0[:])

	receipt := &dbMerkle.Receipt{
		Start: hash0[:],
		Entries: []*dbMerkle.ReceiptEntry{
			{Hash: []byte("right"), Right: true},
		},
	}
	fmt.Printf("  Receipt: Start=%x, Entry.Hash=%x, Entry.Right=%v\n", receipt.Start, receipt.Entries[0].Hash, receipt.Entries[0].Right)

	// 2. Calculate expected root
	fmt.Println("[Step 2] Calculating expected Merkle root...")
	expectedRoot := doSha(append(hash0[:], []byte("right")...))
	fmt.Printf("  expectedRoot: %x\n", expectedRoot)

	// 3. Test FetchProof (mocked)
	fmt.Println("[Step 3] Fetching proof using mock client...")
	cli := &mockLiteClient{mockProof: receipt, mockRoot: expectedRoot}
	va, err := cli.FetchProof(context.Background(), "acc://foo/bar")
	require.NoError(t, err)
	require.NotNil(t, va)
	require.Equal(t, "acc://foo/bar", va.Url)
	require.NotNil(t, va.Receipt)
	fmt.Printf("  Fetched VerifiedAccount: Url=%s, Height=%d\n", va.Url, va.Height)

	// 4. Test VerifyProof (real logic, should succeed)
	fmt.Println("[Step 4] Verifying proof with correct root...")
	realClient := &LiteClient{}
	ok := realClient.VerifyProof(va.Receipt, expectedRoot)
	fmt.Printf("  VerifyProof result (expected true): %v\n", ok)
	require.True(t, ok, "VerifyProof should succeed for valid proof and root")

	// 5. Negative test: wrong root
	fmt.Println("[Step 5] Verifying proof with wrong root...")
	wrongRoot := sha256.Sum256([]byte("wrong"))
	ok = realClient.VerifyProof(va.Receipt, wrongRoot[:])
	fmt.Printf("  VerifyProof result with wrong root (expected false): %v\n", ok)
	require.False(t, ok, "VerifyProof should fail for wrong root")
}
