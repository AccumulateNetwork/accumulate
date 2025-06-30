# Lite Client Architecture

## Overview
The Lite Client is designed to allow users to obtain cryptographically proven account states for a set of accounts, without running a full node. It accomplishes this by securely querying, validating, and tracking proofs and signatures from the Accumulate blockchain.

## Goals
- Efficiently validate account states using cryptographic proofs.
- Verify the integrity of the blockchain from genesis to the current major block.
- Track and validate authority set changes over time.
- Minimize trust in remote nodes by checking all data cryptographically.

---

## High-Level Process

1. **Fetch the Latest Major Block Root Hash**
   - The client starts by fetching the latest major block root hash.

2. **Query and Validate Account Proofs**
   - For each account, the client queries the node for a Merkle proof.
   - The proof is validated by traversing the Merkle Tree Path.
   - Once verified, proofs are cached for efficiency.

3. **Validate Major Block Signatures (Genesis to Current)**
   - The client retrieves the genesis block and its authority set.
   - For each major block, it verifies that the block signatures meet or exceed the required threshold for the authority set active at that block.
   - If any block fails threshold validation, the process halts.

4. **Track Authority Sets Over Time**
   - The client builds an `AuthorityTracker` that maps block indices to `AuthoritySet` instances, capturing the history of authority changes.

5. **Cross-Validation of Authority Sets**
   - The client may fetch and cross-check authorities for each block, ensuring the signatures used are a subset of the authority set at that time.

---

## Key Data Structures

### AuthoritySet
```go
// signatures/authority.go
// Represents the set of signing keys and the threshold for a given block.
type AuthoritySet struct {
	Keys      [][]byte
	Threshold uint64
}
```

### AuthorityTracker
```go
// Maps block indices to their corresponding AuthoritySet.
type AuthorityTracker struct {
	Sets map[uint64]AuthoritySet
}
```

---

## Validation Flow (Go Example)

```go
// client.go (excerpt)
func (c *LiteClient) RetrieveAccountStates(ctx context.Context) error {
	// 1. Fetch the latest major block root hash
	rootHash, err := c.FetchLatestMajorBlockRootHash(ctx)
	if err != nil {
		return err
	}

	// 2. Query and validate account proofs
	proof, err := c.QueryAccountProof(ctx, accountUrl)
	if err != nil || !ValidateMerkleProof(proof, rootHash) {
		return fmt.Errorf("invalid proof")
	}

	// 3. Query all major blocks and extract AuthoritySets
	majorBlocks, err := c.QueryAllMajorBlocks(ctx)
	if err != nil {
		return err
	}
	tracker, err := BuildAuthorityTracker(majorBlocks)
	if err != nil {
		return err
	}

	// 4. Validate block signatures against AuthoritySets
	for idx, block := range majorBlocks {
		set := tracker.Sets[idx]
		if !ValidateBlockSignatures(block, set) {
			return fmt.Errorf("block %d failed signature validation", idx)
		}
	}
	return nil
}
```

---

## Design Notes
- The threshold is stored in each `AuthoritySet` because it is required for validating both block signatures and authority set changes. This is standard practice and matches the reference implementation.
- The `AuthorityTracker` enables efficient lookup of the correct authority set for any block, supporting historical validation and audits.
- All validation is performed using data fetched directly from the network, never fabricated or assumed.

---

## Future Enhancements
- Support for minor block validation and cross-partition proofs.
- Enhanced reporting and error handling.
- Integration with CLI tools for easier user access.

---

## References
- See also: [lite_client.md](./lite_client.md), [authority_validation.md](./authority_validation.md)

---

This document outlines the architecture and rationale for the Lite Client, ensuring robust, auditable, and cryptographically sound validation of account states and blockchain integrity.

