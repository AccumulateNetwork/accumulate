# Changelog

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
