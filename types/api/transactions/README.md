# Transactions
Transactions must be unique (giving them a unique hash and ensuring that replays of transactions are not possible).  This is uniquely difficult where signatures will not be included in the transaction itself.

Design:
Signatures are defined by ADIs.  For Lite Token Chains, the one chain serves as the security, ADI, and token chain.  For everything else, we have

1. ADI (which can have one or more Signature Specification Groups that
control the ADI chain and sub chains)

2. SSG – Signature Specification Group (which can have one or more ordered by
priority KeyPage)

3. KeyPage – Multi Signature Specification (which can have one or more
KeyPage instances)

4. KeyPage – Signature Specification (which validates one or more Sig
instances submitted with transactions)

   * Includes a nonce.  The nonce is incremented with every validated
      signature recorded.  Signatures with a nonce lower that tied to the Signature Specification are rejected, to protect against replay attacks.

5. Sig – Signature (which validates one and only one transaction)

6. transaction (which goes on the chain)

Goal:

Avoid replay attacks by encoding the minimum amount of information in transactions that both make transactions unique, and allow most of the signature information to remain in the pending chain to be pruned from the protocol.

Transaction will include:

1. The Chain URL:  Limits the transaction to the targeted Chain; cannot be
replayed anywhere but on the targeted chain.  The Chain specifies the SSG that must validate the signature.  Limits the transaction to passing validation with the SSG

2. Index of the KeyPage in the SSG as a VarInt.  The Index specifies the
priority of the KeyPage

3. Index of the KeyPage in the KeyPage of the first signature of the
Transaction as a VarInt.  Specifies a use of a KeyPage.

4. Nonce of the KeyPage as a VarInt.  Ensures that the transaction cannot be
replayed (future Sig must have a higher nonce)

Transaction format:

* Header

   * ```<VarInt length> <[] byte(Chain URL)>```

   * ```<VarInt(KeyPage Index in SSG)>```

   * ```<VarInt(KeyPage Index in KeyPage)>```

   * ```<VarInt(nonce)>```

* Transaction

   * ```<VarInt length> <Transaction – format specified by transactions>```

Transaction ID Hash is computed by Hashing the Header, Hashing the Transaction  and hashing the together those hashes. If the transaction is a hash, a Merkle Proof can feed into Accumulate down to its anchors.



