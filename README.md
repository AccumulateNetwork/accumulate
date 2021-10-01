# Accumulate

Accumulate is a novel blockchain network designed to be hugely scalable while maintaining
security. [More details](docs/Accumulate.md)

## CLI

The CLI lives in `./cmd/accumulated`. It can be run directly via `go run ./cmd/accumulated ...`, which builds to a
temporary directory and executes the binary in one go. It can be built via `go build ./cmd/accumualted`, which
creates `accumulated` or `accumulated.exe` in the current directory. Or it can be installed
to `$GOPATH/bin/accumulated` (GOPATH defaults to `$HOME/go`) via
`go install ./cmd/accumulated`.

### Main TestNet

To initialize node configuration in `~/.accumulate`, choose a network from
`networks/networks.go`, and run `accumulated init -n <name>`.

Once `~/.accumulate` is initialized, run `accumulated run -n <n>` where `<n>` is the index of the node you want to run.
For example, "Arches" has two nodes, index 0 and index 1.

### Local TestNet

To set up a testnet on your PC, using localhost addresses, run `accumulated testnet -w <config-dir>`,
e.g. `accumulated testnet -w ./nodes`.

- `-v/--validators` sets the number of nodes created, default 3.
- `--ip` sets the IP address of the first node, default `127.0.1.1`. Nodes after the first will increment the IP,
  e.g. `127.0.1.2`.
- `--port` sets the base port, default `26656`. Tendermint P2P will run on the base port, Tendermint RPC on base port +
  1, Tendermint GRPC on +2, Accumulate RPC on +3, Accumulate JSONRPC API on +4, and Accumulate REST API on +5.

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

### Load Test

To load test an Accumulate network, run `accumulated testnet --network <name>`
or `accumulated testnet --remote <ip-addr>`. These flags can be combined.

- `--network` adds the first IP of the named or numbered network to the target list.
- `--remote` adds the given IP to the target list.
- `--wallets` sets the number of generated wallets.
- `--transactions` sets the number of generated transactions.

