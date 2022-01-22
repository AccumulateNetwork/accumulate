# General Architecture

_This document works its way, loosely, from top to bottom of the flowchart depicted in __./General_Architecture.drawio__.

<span style='color:#88CC88'>

> To assist in illustrating this documentation, green text herein will follow the life of a single transaction which has the following properties:
> - The transaction was initiated by User A, who is the owner of Account A.
> - The transaction is a sending of tokens to Account B (a donation, payment for a purchase, etc), which is owned by User B.
>
> This will follow the transaction in general theory. For a detailed description of how a transaction traces through the Accumulate system, see Transaction_Trace.md.

</span>

## Client Application
One or more users will interact with a client application, which is a DApp or other arbitrary piece of software implementing the Accumulate API. It can be thought of as an Accumulate client instance.  

<span style='color:#88CC88'>

> The user uses their client daemon to execute a transaction. The daemon uses JRPC to send the transaction request to a BVN for processing.  
> _Alternatively:_  
> The user uses an app on their smartphone to execute a transaction. The app sends that request to some server running an ABCI implementation, which then uses JRPC or some other RPC to send the transaction request to a BVN for processing.

</span>

## Routing
A client instance will **route** a transaction to a **BVN** for validation and execution. All client instances have the same routing instructions which determine, based on some properties of the user or transaction, which BVN should receive the transaction. TODO: Who are clients sending their requests to? A node obviously, but how does the client know where the node is?

- <span style='color:#FFAAAA'>**TODO:** Routing instruction management is not currently implemented. Routing instructions are currently hard-coded into the ABCI implementation.  
<code><span style='color:#FFAAAA'>_/internal/api/v2/query_dispatch.go # (q *queryDispatch) direct_</span></code></span>  

- <span style='color:#FFFFAA'>**TODO: VERIFY:** A request sent to the wrong BVN will be refused.  

Routing instructions are currently coded here: <code>_/internal/api/v2/query_dispatch.go_</code>

> <span style='color:#88CC88'>The client instance checks the last several bytes of User A's public key and performs modulo arithmetic in context of the number of available BVN's to choose the correct one.

</span>

## BVN (_Block Validator Network_)
A **BVN** can be thought of as an Accumulate host instance. It manages the receipt, validation, and execution of transactions and other queries. ABCI implementations communicate exclusively with BVN's.  
A BVN consists of multiple nodes which may or may not be distributed across many hosts. Each node performs a discrete function of the BVN. However, as far as anything outside the BVN is concerned (ABCI implementations, other BVN's, etc) the BVN is a single entity.

## Governor
Each BVN has the capacity to act as a **governor** which acts as the moderator for all BVN's in cases where asynchronous communications need to take place. The governor is the BVN which most recently TODO: I forget what the thing it most recently did is supposed to be. Ask Ethan or read the code.  
Currently, these cases are:

- Creation of synthetic transactions.

<span style='color:#88CC88'>

> After the General Executor has validated the transaction and executed a a deduction of value from Account A, the BVN sends a synthetic transaction to the active governor which routes it to the BVN managing User B's account. The synthetic transaction is a simple addition of value to Account B.

</span>

All communications received from another BVN (and which are validated as genuinely coming from that BVN) are trusted implicitly without any validation. For example, synthetic transactions received from another BVN are executed without any validation, because it is assumed that the sending BVN has already performed all of the necessary validation.

</span>

## Tendermint
Tendermint is the mechanism for achieving consensus between BVN's. How Tendermint mechanically achieves this is immaterial and we treat Tendermint as a "black box" for this purpose. However it is important to understand these concepts:

- Each BVN has exactly one Tendermint instance representing it. TODO: Nope.

- Tendermint only cares about its sibling instances agreeing on the hash of a single state machine. For us, that state machine is the _entire_ Accumulate network's stored data and the hash in question is the root hash of the top-level merkle tree storing that data.

- Tendermint is unaware of and does not care about transactions, tokens, ADI's, balances, national elections or your left toe. All it cares about is the aforementioned hash and whether all of its Tendermint siblings agree on that hash.

-  A BVN which consistently fails to achieve consensus with the rest of the BVN's will eventually be kicked out.

Tendermint's solitary role is to provide a simple pass/fail as to whether all the BVN's are in agreement about the state of the database.

## General Executor and Executors
Each BVN has exactly one general executor and many (non-general) executors. Those (non-general) executors each handle one specific type of transaction. The general executor delegates incoming transactions to the appropriate executor.

## Database
The database is the general storage schema for ADI's, balances, and all other information. More documentation on this is available separately.