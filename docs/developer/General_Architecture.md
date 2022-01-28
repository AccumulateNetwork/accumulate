# General Architecture

_This document works its way, loosely, from top to bottom of the flowchart depicted in <code>__./General_Architecture.drawio__</code>._

<span style='color:#88CC88'>

> To assist in illustrating this documentation, green text herein will follow the life of a single transaction which has the following properties:
> - The transaction was initiated by User A, who is the owner of Account A.
> - The other person involved is User B, who owns Account B.
> - User A is sending tokens to User B.
>
> This will follow the transaction in general theory. For a detailed description of how a transaction traces through the Accumulate system, see <code>./Transaction_Trace.md</code>.

</span>

## Client Application
One or more users will interact with a client application, which is a DApp or other arbitrary piece of software implementing the Accumulate API. It can be thought of as an Accumulate client instance.

Each client application is told by its user or its developer where to send requests. Typically the destination will be some central load balancer at a URL such as "mainNet.defidevs.io" from which the request is forwarded to an automatically selected node. It could also be the address of a specific node.

<span style='color:#88CC88'>

> The user uses their CLI client application to execute a transaction. The application uses the Accumulate API to send the transaction request to a node for processing.  
> _Alternatively:_  
> The user uses an app on their smartphone to execute a transaction. The app sends that request to some server running some application, which then uses the Accumulate API to send the transaction request to a node for processing.

</span>

## Routing
A client instance will **route** a transaction to a **BVN** for validation and execution. All client instances have the same routing instructions which determine, based on some properties of the user or transaction, which BVN should receive the transaction. TODO: Who are clients sending their requests to? A node obviously, but how does the client know where the node is?

- <span style='color:#FFAAAA'>**TODO:** Routing instruction management is not currently implemented. Routing instructions are currently hard-coded into the ABCI implementation.  
<code><span style='color:#FFAAAA'>_/internal/api/v2/query_dispatch.go # (q *queryDispatch) direct_</span></code></span>  

- <span style='color:#FFFFAA'>**TODO: VERIFY:** A request sent to the wrong BVN will be refused.  THIS IS FALSE

Routing instructions are currently coded here: <code>_/internal/api/v2/query_dispatch.go_</code>

> <span style='color:#88CC88'>The client instance checks the last several bytes of User A's public key and performs modulo arithmetic in context of the number of available BVN's to choose the correct one.

</span>

## BVN (_Block Validator Network_)
A BVN is a collection of node instances which may or may not be distributed across many physical hosts. Within each BVN there are at least three (qualified) voting nodes and any number of (unqualified) non-voting nodes.

All nodes within a BVN act in parallel and must reach consensus on the results of transactions. When any one node in a BVN receives a transaction request it broadcasts that request to all of its peers in the same BVN. Each node independently processes the transaction and determines a result. The nodes then vote on the results. TODO: verify.

The Accumulate network could theoretically function without BVNs and consist of a huge collection of nodes all in one network. The advantage of a BVN is the subdivision of work.  
Suppose the entire Accumulate ecosystem consists of 5,000 nodes. Without BVNs, when a transaction occurs <i>all</i> of the 5,000 nodes must validate it and then vote on it. If these nodes are instead subdivided into 500 BVNs of 10 nodes each, a maximum of 20 nodes will be involved in the transaction. This saves work and network traffic.

Note that the division of the Accumulate ecosystem into BVNs is purely logical. TODO: How are the specifics of this division determined?

For detailed information about nodes, see <code>./Launch_Detail.md</code>.

## Governor
TODO: Where does the governor function run? On all nodes in the governor BVN? Or one specific node?  
Each BVN has the capacity to act as the governor which acts as the moderator for all BVN's in cases where asynchronous communications need to take place. The governor is the BVN which most recently TODO: I forget what the thing it most recently did is supposed to be. Ask Ethan or read the code.  
Currently, these cases are:

- Creation of synthetic transactions.

<span style='color:#88CC88'>

> After the General Executor has validated the transaction and executed a a deduction of value from Account A, the BVN sends a synthetic transaction to the active governor which routes it to the BVN managing User B's account. The synthetic transaction is a simple addition of value to Account B.

</span>

All communications received from another BVN (and which are validated as genuinely coming from that BVN) are trusted implicitly without any validation. For example, synthetic transactions received from another BVN are executed without any validation, because it is assumed that t0he sending BVN has already performed all of the necessary validation.

</span>

## Tendermint
Tendermint is the mechanism for achieving consensus between BVNs. How Tendermint mechanically achieves this is immaterial and we treat Tendermint as a "black box" for this purpose. However it is important to understand these concepts:

- Tendermint only cares about its sibling instances agreeing on the hash of a single state machine. For us, that state machine is the _entire_ Accumulate network's stored data and the hash in question is the root hash of the top-level merkle tree storing that data.

- Tendermint is unaware of and does not care about transactions, tokens, ADI's, balances, or anything else related to Accumulate or blockchain. All it cares about is the aforementioned hash and whether all of its Tendermint siblings agree on that hash.

-  A BVN which consistently fails to achieve consensus with the rest of the BVN's will eventually be kicked out of the voting process.

Tendermint's solitary role is to provide a simple pass/fail as to whether all the BVN's are in agreement about the state of the database.

## General Executor and Executors
Each node has exactly one general executor and many sub-executors. Those sub-executors each handle one specific type of transaction. The general executor delegates incoming transactions to the appropriate sub-executor.

## Database
The database is the general storage schema for ADI's, balances, and all other information. More documentation on this is available separately.