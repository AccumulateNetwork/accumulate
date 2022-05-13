# Principals of Determinism

Goals:

- The system, as represented by the ABCI/executor, is completely deterministic.
- The only operations that affect the state are InitChain, BeginBlock,
  DeliverTx/ExecuteEnvelope, EndBlock, and Commit.
- The state at any given point in time is fully determined by the sequence of
  calls (and their arguments).
- The state is completely independent of the timing of these calls.
- The BPT hash and the root chain anchor are a complete summary of the system
  state and history.

In order to achieve these goals, we must enforce the following principals:

- State must not be mutated outside of calls to InitChain, BeginBlock,
  DeliverTx/ExecuteEnvelope, EndBlock, and Commit
- State mutation must be fully determined by the order of calls and their
  arguments.
- Any change to the state of an account ***must*** be accompanied by adding an
  entry to one of the account's chains.
- Any change to the state of a transaction ***must*** be accompanied by adding
  an entry to a chain of an account.