// This file defines the logic for generating and verifying Merkle proofs of account inclusion
// in the Accumulate blockchain’s Binary Patricia Tree (BPT). These proofs allow external
// clients (like a lite client) to verify account existence in a particular state root.
//
// ## Key Concepts:
// - Each account in Accumulate is stored in the BPT with a unique hash-based key.
// - A Merkle proof (receipt) shows how a specific account hash links up to the BPT root hash.
// - This logic builds such a proof and optionally verifies it using only cryptographic hashes.
//
// ## Dependencies:
// - `internal/database/Batch` provides access to the state BPT.
// - `pkg/types/merkle` provides receipt and hashing utilities.
// - `pkg/url` supports validated parsing of Accumulate URLs.
// - `record.NewKey` constructs a BPT lookup key from the account URL.

package liteclient

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// AccountProof represents a Merkle proof of inclusion for a specific account URL.
// The proof can be validated independently using only hashes and root data.
type AccountProof struct {
	AccountUrl string   // The Accumulate-formatted account URL (e.g., acc://foo/bar)
	LeafHash   []byte   // The leaf hash of the account data
	Siblings   [][]byte // The Merkle sibling hashes forming the proof path
	RootHash   []byte   // The target root hash that this path must match
	RootIndex  int64    // The block index where this root was committed
}

// CreateAccountProof builds a Merkle receipt for the provided account URL and
// returns the relevant proof elements in a portable struct.
func CreateAccountProof(batch *database.Batch, accountUrl string) (*AccountProof, error) {
	// Validate input parameters
	if batch == nil {
		return nil, fmt.Errorf("batch cannot be nil")
	}
	if accountUrl == "" {
		return nil, fmt.Errorf("account URL cannot be empty")
	}

	// 1. Parse and validate the input string
	u, err := accurl.Parse(accountUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL %q: %w", accountUrl, err)
	}

	// 2. Build the Merkle receipt for the account's key in the BPT
	key := record.NewKey("Account", u)
	receipt, err := batch.BPT().GetReceipt(key)
	if err != nil {
		return nil, fmt.Errorf("failed to get BPT receipt for account %v: %w", u, err)
	}
	if receipt == nil {
		return nil, fmt.Errorf("no receipt found for account %v", u)
	}

	// 3. Extract leaf hash and sibling hashes from the receipt
	if receipt.Start == nil {
		return nil, fmt.Errorf("invalid receipt: missing start hash for account %v", u)
	}
	leafHash := receipt.Start
	siblings := extractSiblingsFromReceipt(receipt)

	// 4. Fetch the current BPT root hash from the batch
	root, err := batch.GetBptRootHash()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve BPT root hash for account %v: %w", u, err)
	}
	if root == ([32]byte{}) {
		return nil, fmt.Errorf("invalid BPT root hash for account %v", u)
	}

	// 5. Return the constructed proof
	return &AccountProof{
		AccountUrl: accountUrl,
		LeafHash:   leafHash,
		Siblings:   siblings,
		RootHash:   root[:],
		RootIndex:  receipt.EndIndex,
	}, nil
}

// extractSiblingsFromReceipt extracts the sibling hashes from a Merkle receipt.
// Each entry represents a hash in the Merkle path (either left or right sibling).
func extractSiblingsFromReceipt(receipt *merkle.Receipt) [][]byte {
	siblings := make([][]byte, len(receipt.Entries))
	for i, entry := range receipt.Entries {
		siblings[i] = entry.Hash
	}
	return siblings
}

// VerifyAccountProof recomputes the hash path from the leaf through the siblings
// and checks if it leads to the given root hash.
func VerifyAccountProof(p *AccountProof) bool {
	if p == nil {
		return false
	}
	if p.LeafHash == nil || len(p.LeafHash) != 32 {
		return false
	}
	if p.RootHash == nil || len(p.RootHash) != 32 {
		return false
	}

	return VerifyBptProof(p.LeafHash, p.Siblings, p.RootHash)
}
