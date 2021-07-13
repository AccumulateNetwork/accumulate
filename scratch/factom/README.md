# `package factom`

[![GoDoc](https://godoc.org/github.com/Factom-Asset-Tokens/factom?status.svg)](https://godoc.org/github.com/Factom-Asset-Tokens/factom)

Package factom provides fundamental types for the Factom blockchain, and uses
the [factomd JSON-RPC 2.0 API](https://docs.factom.com/api#factomd-api) to
populate data.

This package was originally created as part of the [`Factom Asset Token
Daemon`](https://github.com/Factom-Asset-Tokens/fatd) to replace [`package
github.com/FactomProject/factom`](https://github.com/FactomProject/factom).

## Features
- Oriented around the [Factom data
  types](https://docs.factomprotocol.org/start/factom-data-structures), not the
factomd API
- UnmarshalBinary and MarshalBinary implemented for all Factom data structure
  types
- Load a DBlock by Height or KeyMR
- Load an EBlock by KeyMR and ChainID, or load the latest EBlock for a ChainID
- Load an Entry by Hash
- Create a new Entry for an existing ChainID or create the first Entry of a new
  chain
- Work with FA/FsAddresses and EC/EcAddresses
- Load an Identity and its IDKeys
- Work with ID1-4Keys

## Contributing

This repo is heavily influenced by the [`Factom Asset Token
Daemon`](https://github.com/Factom-Asset-Tokens/fatd) repo. As such the
[CONTRIBUTING.md](https://github.com/Factom-Asset-Tokens/fatd/blob/develop/CONTRIBUTING.md)
and
[CODE_POLICY.md](https://github.com/Factom-Asset-Tokens/fatd/blob/develop/CODE_POLICY.md)
files from that repo are required reading for contributing.
