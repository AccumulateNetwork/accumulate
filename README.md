# Accumulate

Accumulate is a novel blockchain network designed to be hugely scalable while
maintaining security. [More details](docs/Accumulate.md)

## CLI

### Main TestNet

To initialize node configuration in `~/.accumulate`, choose a network from
`networks/networks.go`, and run `accumulated init -n <name>`.

Once `~/.accumulate` is initialized, run `accumulated run -n <n>` where `<n>` is
the index of the node you want to run. For example, "Arches" has two nodes,
index 0 and index 1.

### Local TestNet

To set up a testnet on your PC, using localhost addresses, run `accumulated
testnet -w <config-dir>`, e.g. `accumulated testnet -w ./nodes`.

- `-v/--validators` sets the number of nodes created, default 3.
- `--ip` sets the IP address of the first node, default `127.0.1.1`. Nodes after
  the first will increment the IP, e.g. `127.0.1.2`.
- `--port` sets the base port, default `26656`. Tendermint P2P will run on the
  base port, Tendermint RPC on base port + 1, Tendermint GRPC on +2, Accumulate
  RPC on +3, Accumulate JSONRPC API on +4, and Accumulate REST API on +5.

To run a node in the testnet, run `accumulated run -w <dir>/Node<n>`, e.g.
`accumulated run -w ./nodes/Node0`.

## Code organization

Accumulate is broken into the following components:

- `cmd/accumulated` - CLI
- `internal/abci` - Tendermint ABCI application
- `internal/chain` - Accumulate chain validation and processing
- `internal/node` - Tendermint node configuration, initialization, and execution
- `internal/relay` - The relay, responsible for relaying transactions to the appropriate BVC
- `router` - Accumulate API
- `smt` - Stateful Merkle Tree
- `types` - Data type definitions, used for RPC and persistence
