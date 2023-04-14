# Signing

## Constructing Envelopes

- Transactions are submitted to the protocol within envelopes.
- An envelope is comprised of signatures and a transaction.
- A transaction is comprised of a principal account, initiator hash, and body.

## Transaction Hashes

- If the envelope specifies a transaction hash it will be used by any signatures
  and/or remote transactions that do not themselves include a transaction hash.
- Otherwise, if the envelope only has a single transaction its hash will be used
  by any signatures that do not themselves include a transaction hash.
- All remote transactions in the envelope must themselves include a transaction
  hash unless the envelope specifies a transaction hash.
- All signatures in the envelope must themselves include a transaction hash
  unless the envelope specifies a transaction hash or only has a single
  transaction.

## Signing Transactions

- Signatures for a transaction can be split across multiple envelopes.
- The first signature of a transaction is the initiator.
- The transaction's initiator hash must equal a simple or merkle hash of the
  initiator's metadata¹.
- Multiple types of signatures are supported, e.g. ED25519 and RCD1.
- Signatures are comprised of the public key, signer URL, signer version, a
  timestamp (optionally), and the actual signed hash.
- A timestamp is required for the initiator.
- The initiator may add a salt to the signer URL, e.g.
  `my/book/1?salt=cafeb0ba`, to frustrate attempts to determine who initiated
  the transaction (once the signatures expire).

¹The metadata of a signature includes all of the signatures fields except the
actual signed hash

## Constructing a Signature

1. Setup the signature object with all of the fields except the actual signed
   hash.
2. Marshal and hash the signature (sans signature) = `sigHash`.
3. For the first signature, set the transaction's initiator hash to `sigHash`.
4. Marshal and hash the transaction header = `txnHeaderHash`.
5. Marshal and hash the transaction body = `txnBodyHash`.
6. Construct the hash to be signed:
   - Legacy ED25519: `H(sigHash | uvarint(timestamp) | H(txnHeaderHash | txnBodyHash))`
   - ED25519, RCD1: `H(sigHash | H(txnHeaderHash | txnBodyHash))`
7. Sign the hash.

## Authorization & Collecting Signatures

- Most accounts can be governed by multiple authorities.
- Lite token accounts are governed by themselves and do not support advanced
  auth.
- The authorities of a (non-lite) account can be enabled or disabled
  individually.
- Key pages inherit the governing authorities of the key book they belong to.
- Key books and lite token accounts are authorities.
- Key pages and lite token accounts are signers.
- A transaction's signatures are grouped into signature books and pages by
  authority and signer, respectively.
- Each key of a signer can respond at most once. Additional responses
  (signatures) from the same public key are ignored.
- A transaction is ready to execute once all of the enabled authorities of the
  transaction's principal account are satisified.
- An authority is satisfied once any of its signers are satisfied.
- A signer is satisfied once the corresponding signature page is complete.
- A signature page is complete once it meets the signature threshold of the
  corresponding signer.
- If all authorities of the principal are disabled, the transaction still
  requires single signature (from any account).
