# Accumuate API

## Architecture

### Modules

The Accumulate API is segmented into the following modules:

* The Network module provides methods for querying network metadata such as TPS
* The Node module provides methods for querying node metadata such as the node's
  software version.
* The Query module provides methods for querying records.
* The Submit module provides methods for submitting transactions.

### Layers

The Accumulate API is composed of the following layers:

* The Network layer executes Network requests by collating information from all
  network partitions.
* The Node layer executes Node requests for a node.
* The Database Query layer executes Query requests using a database.
* The Tendermint Submit layer executes Submit requests by forwarding them to
  Tendermint RPC.
* The Router layer executes Query and Submit requests by forwarding them to the
  appropriate network partition or data server.

### Implementations

* JSON-RPC
* Binary
* REST?

## Methods

### Query

* `QueryState(account, fragment..., options)` queries the state of an account or
  an account fragment such as a transaction or other chain entry. QueryState can
  query at most one record at a time. Queries involving multiple records must
  use QuerySet.
* `QuerySet(account, fragment..., options)` queies a set of records relating to
  an account, such as directory entries, transactions, etc.
* `Search(scope, recordQuery, options)` searches for a record within the scope
  of an account or identity. For example, Search can locate a transaction on the
  main chain of any account within an ADI.

### Submit

* `Submit(envelope, options)` submits a transaction for execution. Submit can
  also be used to validate a transaction without submitting it.
* `Faucet(account, options)` funds a lite account (for testnet only).

### Network

* `Metrics()` collects metrics from the entire network

### Node

* `Status()` returns the status of the node
* `Version()` returns the software version of the node
* `Describe()` returns the network configuration of the node
* `Metrics()` collects metrics from the node