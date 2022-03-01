# Accumulate

Accumulate is a novel blockchain network designed to be hugely scalable while maintaining
security. [More details](docs/Accumulate.md)

## Command Line Interface (CLI)

The CLI lives in `./cmd/accumulate`. It can be run directly via `go run ./cmd/accumulate`, which builds to a
temporary directory and executes the binary in one go. It can be built via `go build ./cmd/accumulate`, which
creates `accumulate` or `accumulate.exe` in the current directory. It can be installed to `$GOPATH/bin/accumulate` (
GOPATH defaults to `$HOME/go`) via
`go install ./cmd/accumulate`.

### Main TestNet

To initialize node configuration in `~/.accumulate`, choose a network from
`networks/networks.go`, and run `accumulated init -n <name>`.

Once `~/.accumulate` is initialized, run `accumulated run -n <n>` where `<n>` is the index of the node you want to run.
For example, "Arches" has two nodes, index 0 and index 1.

### Local TestNet

Before a local testnet can be run, you need to initialize some devnet configuration files for a few nodes. 


- `-v/--validators` sets the number of nodes created, default 3.
- `--ip` sets the IP address of the first node, default `127.0.1.1`. Nodes after the first will increment the IP,
  e.g. `127.0.1.2`.
- `--port` sets the base port, default `26656`. Tendermint P2P will run on the base port, Tendermint RPC on base port +
  1, Tendermint GRPC on +2, Accumulate RPC on +3, Accumulate JSONRPC API on +4, and Accumulate REST API on +5.

```bash
# The config files will be placed in $HOME/.accumulate/dn
accumulated init devnet -v 3 --ip 127.0.0.1 --port 26656
```

From here you can launch the devnet. Optionall change the config directory with `-w <config-dir>`

```bash
accumulated run devnet
```

To run a node in the testnet, run `accumulated run -w <dir>/Node<n>`, e.g.
`accumulated run -w ./nodes/Node0`.

## Code organization

Accumulate is broken into the following components:

- `cmd/accumulated` - Daemon
- `cmd/accumulate` - CLI wallet
- `config` - Daemon configuration types
- `internal/abci` - [ABCI](https://docs.tendermint.com/master/spec/abci/)
  implementation
- `internal/api`- JSON-RPC v1; **Deprecated**: use `internal/api/v2`
- `internal/api/v2`- JSON-RPC v2
- `internal/chain` - Transaction validation and processing (aka chain
  validators)
- `internal/cmd/gentypes` - Type generator for `internal/api` and `protocol`
- `internal/encoding` - Shared code used by generated types in `internal/api`
  and `protocol`
- `internal/genesis` - Objects that are created when the node first starts up
- `internal/logging` - Logging setup
- `internal/node` - Node configuration, initialization, and execution
- `internal/relay` - The relay, responsible for relaying transactions to the
  appropriate BVC; **Deprecated**: use `internal/api/v2`
- `internal/testing` - Test helpers
- `internal/url` - Accumulate URL parser
- `internal/web/static` - Embedded web server content
- `networks` - Accumulate network definitions
- `protocol` - Protocol types and definitions
- `router` - Accumulate API
- `smt` - Stateful Merkle Tree
- `types` - Data type definitions, used for RPC and persistence; **Deprecated**:
  these packages will eventually be moved or replaced, new work should happen
  elsewhere

### Load Test

To load test an Accumulate network, run `accumulated testnet --network <name>`
or `accumulated testnet --remote <ip-addr>`. These flags can be combined.

- `--network` adds the first IP of the named or numbered network to the target list.
- `--remote` adds the given IP to the target list.
- `--wallets` sets the number of generated wallets.
- `--transactions` sets the number of generated transactions.

