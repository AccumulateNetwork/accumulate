# Verification Process

This document explains the process of verifying anchor proofs and receipts in Accumulate.

## Verification Steps

The verification of an anchor proof involves the following steps:

1. **Validate the proof structure**
   - Ensure all required fields are present
   - Check that hashes are the correct length and format

2. **Verify the Merkle path**
   - Start with the transaction hash
   - Apply the sibling hashes according to the path
   - Confirm the resulting hash matches the root hash

3. **Verify the anchor inclusion**
   - Confirm the root hash is included in the anchor
   - Verify the anchor is included in the target chain
   - Check the block height and timestamp

4. **Validate chain references**
   - Ensure the source and destination networks are valid
   - Verify the cross-chain references are consistent

## Error Handling

During verification, several errors may occur:

- Invalid proof format
- Merkle path verification failure
- Anchor not found in target chain
- Timestamp inconsistencies
- Network reference errors

Each error is handled with specific error codes and detailed messages to aid in debugging.

## Performance Considerations

Verification is designed to be efficient:

- Caching of verified anchors
- Batch verification when possible
- Early termination on failure
- Optimized hash calculations
