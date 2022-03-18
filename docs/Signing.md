# Signing

- Transactions are submitted to the protocol within envelopes
- The envelope defines the principal account of the transaction
- The envelope includes signatures
- The first signature is the 'initiator'
- A signature must specify the signature type, e.g. ED25519 or RCD1
- A signature must include the signed hash, i.e. the actual signature
- A signature must include the public key or script used to verify the signature
- A signature must include the URL of the signer, i.e. the key page or lite token account the public key belongs to
- A signature must include the signer version, if the signer is a key page
- The initiator must include a timestamp
- Optionally, the initiator may add a salt to the signer URL to obfuscate it, e.g. `my/book/1?salt=cafeb0ba`
- To save space and obfuscate the initiator, the transaction includes the initiator hash, but does not include any initator details
- The initiator hash is calculated from the public key, signer URL, signer version, and timestamp
- The transaction hash is calculated as `H(H(txn header) . H(txn body))`

Include the principal URL in the initiator hash? Shouldn't be needed, because
the timestamp prevents replay.