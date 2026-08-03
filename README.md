[![Go Reference](https://pkg.go.dev/badge/gitlab.com/accumulatenetwork/accumulate.svg)](https://pkg.go.dev/gitlab.com/accumulatenetwork/accumulate)

# Accumulate

Accumulate is a novel blockchain network designed to be hugely scalable while
maintaining security.

- Wallets & Apps
  - [Explorer](https://explorer.accumulatenetwork.io/)
  - [Mobile](https://accumulatenetwork.io/wallet/)
  - [CLI](https://gitlab.com/accumulatenetwork/core/wallet)
- Documentation
  - [User documentation](https://docs.accumulatenetwork.io/accumulate/)
  - [Architecture](https://docs.accumulatenetwork.io/core/)

## Repository organization

### Commands

- `cmd/accumulated` initializes and runs node daemons.
- `cmd/accumulated-bootstrap` runs a libp2p bootstrap node.
- `cmd/accumulated-faucet` runs a token faucet.
- `cmd/accumulated-http` runs an HTTP API node.

### SDK

- `pkg/api/v3` contains specifications for API v3 and transport implementations.
- `pkg/build` provides utilities for building and signing transactions.
- `pkg/client/api/v2` is a JSON-RPC client for API v2.
- `pkg/errors` defines Accumulate's status and error codes and a serializable
  error implementation.
- `pkg/types` contains the protocol's data types.
- `pkg/url` implements custom types for Accumulate URLs and transaction/message
  IDs.
- `protocol` contains additional protocol data types and will eventually be
  migrated to `pkg/types`.

### Experimental

Experimental packages should be used with caution and may change without notice.

- `exp/light` is a light client prototype.
- `vdk` is a validator development kit prototype.

### Internal

Internal packages should not be used outside of this repository.

- `internal/api` contains API service implementations.
- `internal/bsn` is the block summary network (prototype).
- `internal/core` is the core executor system.
- `internal/database` is the core data model.
- `internal/node` is plumbing for the node daemon.

### Testing

Testing packages are primarily intended for testing this repository. A few are
stable enough to be used externally.

- `test/harness` is a harness for building Go tests against an Accumulate
  network. The harness can be used to test a simulated network or a real network
  and exposes the same interface for both. The network **must** support API v3.
- `test/simulator` is an Accumulate network simulator, capable of running
  multiple partitions with multiple nodes each with a basic consensus mechanism.
  The simulator must be stepped manually or via a timer.
- `test/testing` is deprecated and should not be used by new tests.

The other packages are either nothing but tests or not intended for external use
and not guaranteed to be stable.

### Tools

These tools were not designed to be used externally and may change without
notice. Use at your own risk.

- `tools/cmd/debug` is a collection of debugging utilities.
- `tools/cmd/explore` is an experimental database explorer.
- `tools/cmd/factom` converts Factom database dumps to snapshots for genesis.
- `tools/cmd/gen-api` generates JSON-RPC API implementations.
- `tools/cmd/gen-enum` generates enumeration types.
- `tools/cmd/gen-model` generates data models.
- `tools/cmd/gen-sdk` generates JSON-RPC API clients.
- `tools/cmd/gen-types` generates serializable data types.
- `tools/cmd/genesis` is a collection of utilities for building snapshots for
  genesis.
- `tools/cmd/golangci-lint` is a fork of golangci-lint that includes custom
  linters.
- `tools/cmd/repair-indices` rebuilds database indices for a core node.
- `tools/cmd/resend-anchor` scans the network and resends anchor transactions
  and signatures.
- `tools/cmd/sendinterrupt` does the equivalent to sending Ctrl-C to a Windows
  console window.
- `tools/cmd/simulator` simulates an Accumulate network.
- `tools/cmd/snapshot` is a collection of utilities for database snapshots.