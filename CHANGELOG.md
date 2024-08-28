# Changelog

## 1.4

- Protocol
  - Support for EIP-712 (typed data) signatures
  - Support for PKIX RSA signatures
  - Support for PKIX ECDSA signatures
  - Support for AC2/AS2 (PKIX ECDSA), AC3/AS3 (PKIX RSA), and WIF addresses
  - Suggested transactions
  - Discount creation of bare ADIs (without a key book)
  - Discount creation of sub-ADIs
- API
  - Support for retrieving a receipt at a specific height
  - Support appending an epilogue to a transaction
  - Support minimum viable Ethereum RPC for MetaMask
- Node operations
  - New and improved node configuration framework
  - Progress towards snapshot sync
  - Support adding an index to a snapshot
  - Support for Bolt and LevelDB
  - Exploratory custom database implementation to improve TPS
- Network operations
  - Improve peer-to-peer service discovery
  - Improve reliability of healing
  - Improve performance of light client indexing
  - Decouple ACME burns (for credits) from anchors
  - Decouple network updates (e.g. the oracle) from anchors
  - Decouple major blocks from anchors
  - Improve reliability of major blocks
  - Improve reliability of anchoring
  - Improve API stability
- Other
  - Reduce overhead of BPT hash calculations
  - Redesign the simulator's consensus model
  - Fix lite account authority handling bugs

## 1.3

- Protocol
  - User-specified transaction expiration
  - Rejection of invalid authorities
  - Dynamic inheritance of authorities
  - Additional transaction authorities
  - Reduced cost for creating sub-ADIs
  - Memos and metadata for signatures
  - RSV Ethereum signatures (deprecates DER)
  - Database performance improvements
  - Prevent persistence of bad blocks
  - Bug fixes
- Operations
  - Anchoring improvements
  - Enable snapshot v2
  - Use binary genesis file for new nodes
- SDK
  - Embedded checkpoint for validating network state

## 1.2.10

- API
  - Query responses now include `lastBlockTime`, which is retrieved from the
    consensus layer. This can be used to ensure the response is up to date.
  - Sub-records of query responses (those that have sub-records) now may be
    replaced with an error record, if the sub-record cannot not be found. This
    prevents the entire query from failing if a sub-record cannot be found.
  - Service discovery methods were tweaked to facilitate routing improvements.
  - Refactored request routing to improve API stability.
  - Added a REST API.
- Operations
  - Improved Prometheus metrics.
  - Improved snapshot performance.
  - Improved dispatch for anchors and synthetic transactions.
  - Fixed a bug with the Badger database driver.

## 1.2

- Signature processing overhaul
- Rejections and abstentions
- Transaction review periods
- Snapshot v2
- API improvements

## 1.1.3

- Fixes a bug that can lead to unresolvable synthetic transactions (#3351, !865)

## 1.1 (.2)

1.1.0 was retracted due to a consensus bug. 1.1.1 was retracted due to user
error (the wrong commit was tagged).

### Configuration changes

```toml
##### Required (DNN only) #####

# Add a new section
[p2p]
  # Bootstrap peers for connecting to Accumulate's libp2p network
  bootstrap-peers = [
    "/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg",
  ]
  # API v3/libp2p listening addresses
  listen = [
    "/ip4/0.0.0.0/tcp/16593",
    "/ip4/0.0.0.0/udp/16593/quic",
  ]

##### Recommended (DNN and BVNN) #####

# Update the existing section
[snapshots]
  # Disable snapshot collection to reduce resource consumption
  enable = false
```

### API

- For certain types, zero-valued fields will be omitted from JSON output instead
  of being returned as null or zero.
  - `sendTokens.hash`, `signature.transactionHash`, `tokenIssuer.issued`,
    `dataAccount.entry`

## 1.0.4

- Replace Accumulate data entries with double hash data entries and reject
  transactions with bodies that are exactly 64 bytes to resolve a potential
  weakness in the security of communications between network partitions (#3283,
  !810)

## 1.0.3

- Allow the latest protocol version to be reactivated (#3228, !754)

## 1.0.2

- Implement versioning of the core executor code (#3152, !684)
  - Fixes a bug where the version change network update is not published to BVNs
  - Logs an error if multiple database batches concurrently change the same
    value
- Anchors signature chains into the root chain (#3149, !681)
- Miscellaneous fixes and changes (#3154, !685)
  - Fixes an issue with recording signatures
  - Fixes improper forwarding of synthetic transactions
  - Allows updates to the authority set of network accounts
  - Fixes an issue with recording the transaction initiator
  - Rejects malformed envelopes
  - Stops adding empty burns to the ACME token issuer
- Fixes a bug that could lead to global consensus failure due to a faulty error
  message (#3157, !689)

## 1.0.1

- Fix bugs in the SDK (617ff4673919aa0f17596ba2702ee075daca4a3c, 95694666ef9d562497bd43cbb9473533170f9be4)
- Fix error reporting in the ABCI (#49, !657)
- Add a way to determine the status of remote multisig transactions (#50, !658)
