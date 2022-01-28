# Transaction Trace

_This document describes in detail the movement of a single transaction through the Accumulate system. For a less-detailed, more macroscopic and theoretical description of the system, see General_Architecture.md._

Transactions begin at the top of this table and move downward. More detailed descriptions of each step are available below.

If an error or failure to validate occurs on any step, downward execution stops immediately.

This table will follow a theoretical "add-credits" transaction.

<table>
    <thead>
        <tr>
            <th colspan='2'>Step</th>
            <th>Method...</th>
            <th>...executed from file...</th>
            <th>...which is an implementation of...</th>
            <th>...running on...</th>
            <th>...as part of:</tha>
        </tr>
    </thead>
    <tbody>
        <tr>
            <td rowspan='4'><b>S<br>E<br>N<br>D<br></b></td>
            <td style='width:3em;text-align:center'>1</td>
            <td><code>AddCredits</code></td>
            <td><code>/cmd/accumulate/cmd/credits.go</code></td>
            <td rowspan='4'>An arbitrary client program.</td>
            <td rowspan='4'>A client device: a CLI, or a DApp backend, etc.</td>
            <td rowspan='4'>The user.</td>
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>2</td>
            <td><code>dispatchTXRequest</code></td>
            <td><code>/cmd/accumulate/cmd/util.go</code></td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>3</td>
            <td><code>Request</code></td>
            <td><code>/client/client.go</code></td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>4</td>
            <td><code>Request</code></td>
            <td>[ The JSONRPC2 black box ]</td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <td rowspan='0'><b>R<br>E<br>C<br>E<br>I<br>V<br>E</td>
            <td style='text-align:center'>5</td>
            <td><code>ExecuteAddCredits</code></td>
            <td><code>/internal/api/v2/api_gen.go</code></td>
            <td rowspan='8'>A JSON RPC listener,<br>as defined in<br><code>./jrpc.go</code></td>
            <td rowspan='6'>An arbitrary host running a BVN node daemon instance.</td>
            <td rowspan='6'>An arbitrary BVN.</td>
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>6</td>
            <td><code>executeWith</code></td>
            <td rowspan='3'><code>/internal/api/v2/jrpc_execute.go</code>
            </td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>7 - A</td>
            <td><code>execute</code></td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>7 - B</td>
            <td><code>executeBatch</code></td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>7 - C</td>
            <td><code>CallBatch</code></td>
            <td>[ The JSONRPC2 black box ]</td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>7 - D</td>
            <td><code>Execute</code></td>
            <td rowspan='3'><code>/internal/api/v2/jrpc_execute.go</code></td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>7 - E</td>
            <td><code>execute</code></td>
            <!-- col -->
            <td rowspan='7'>A host running a BVN node daemon instance, guaranteed to be within the correct BVN.</td>
            <td rowspan='0'>The correct destination BVN for the given transaction.</td>
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>8</td>
            <td><code>executeLocal</code>
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>9</td>
            <td><code>BroadcastTxSync</code></td>
            <td><code>/internal/node/local_client.go</code></td>
            <td rowspan='3'>A Tendermint client.</td>
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>10</td>
            <td><code>BroadcastTxSync</code></td>
            <td>[ The Tendermint black box ]</td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>11 - A</td>
            <td><code>CheckTx</code></td>
            <td><code>/internal/abci/accumulator.go</code></td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>11 - B</td>
            <td><code>CheckTx</code></td>
            <td><code>/internal/chain/executor_txn.go</code></td>
            <td>An executor,<br>as defined in<br><code>./executor.go</code></td>
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>11 - C</td>
            <td><code>Validate</code></td>
            <td><code>/internal/chain/add-credits.go</code></td>
            <td>A sub-executor,<br>as defined in<br><code>./chain.go</code></td>
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>12 - A</td>
            <td><code>CheckTx</code></td>
            <td><code>/internal/abci/accumulator.go</code></td>
            <td>A Tendermint client.</td>
            <td rowspan='0'><b>ALL</b> peers of the host which performed steps 7-E through 11, within the correct BVN, running BVN node daemon instances.</td>
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>12 - B</td>
            <td><code>CheckTx</code></td>
            <td><code>/internal/chain/executor_txn.go</code></td>
            <td>An executor,<br>as defined in<br><code>./executor.go</code></td>
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>12 - C</td>
            <td><code>Validate</code></td>
            <td><code>/internal/chain/add-credits.go</code></td>
            <td>A sub-executor,<br>as defined in<br><code>./chain.go</code></td>
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>13 - A</td>
            <td><code>DeliverTx</code></td>
            <td><code>/internal/abci/accumulator.go</code></td>
            <td>A tendermint client.</td>
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>13 - B</td>
            <td><code>DeliverTx</code></td>
            <td><code>/internal/chain/executor_txn.go</code></td>
            <td>An executor,<br>as defined in<br><code>./executor.go</code></td>
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>13 - C</td>
            <td><code>Validate</code></td>
            <td><code>/internal/chain/add-credits.go</code></td>
            <td>A sub-executor,<br>as defined in<br><code>./chain.go</code></td>
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>13 - D</td>
            <td><code>putTransaction</code></td>
            <td><code>/internal/chain/executor_txn.go</code></td>
            <td rowspan='0'>An executor,<br>as defined in<br><code>./executor.go</code></td>
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>14</td>
            <td><code>Commit</code></td>
            <td><code>/internal/chain/executor.go</code></td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr>
    </tbody>
</table>

## Notes

1. In the CLI client implementation provided by DeFiDevs, this method is triggered by a user-entered command.

2. All of the specifics of the transaction request were handled in step 1. Now they are being packed up for sending.

3. The request is handed off to the JSONRPC client.

4. The request is processed by the JSONRPC client and actually sent. This is the final departure from the client.

5. The JSONRPC2 black box selects a method from <code>/internal/api/v2/api_gen.go</code> to execute based on the "action" sent along with they payload, defined in step 1 and actually provided to JSONRPC2 in step 3. This is the entry point of execution on the node ("server") side.

6. The content of the transaction request is unpacked for the first time. The relevant payload is forwarded to step 7.

7. &nbsp;

    <ol type='A'>
        <li><!-- A -->Routing instructions are consulted:
            <ul>
                <li>If the transaction is to be executed locally (this node is in the correct BVN) then skip to step 8.</li>
                <li>If the transaction is not to be executed locally, then proceed to step 7-B.</li>
            </ul>
        </li>
        <li><!-- B -->The node is added to a batch of outgoing transaction requests for tranmission to a node on another BVN.  TODO: How do we determine which node?</li>
        <li><!-- C -->The batch is sent via the JSONRPC2 black box to a node on another BVN.</li>
        <li><!-- D -->The JSONRPC2 black box calls <code>Execute</code> on a node inside the correct BVN.</li>
        <li>The same <code>execute</code> method from step 7-A is repeated, but this time guaranteed to proceed to step 8 because this node is necessarily in the correct BVN for this transaction.<br>&nbsp;</li>
    </ol>  

8. The transaction is prepared for sending to Tendermint.

9. The transaction is handed over to Tendermint.

10. Tendermint begins the process of soliciting consensus on the validity of the transaction. Steps 11, 12, 13, and 14 are all called by the Tendermint black box.

11. Before getting any other nodes involved, Tendermint turns around and instructs the calling node to validate the transaction itself. If the calling node doesn't think the transaction is valid, then there is no point in spending resources having all the node's peers also try to validate the transaction.

    <ol type='A'>
        <li><!-- A -->The transaction is prepared for handing down to the node's executor.</li>
        <li><!-- B -->The transaction is handed down to the node's executor which checks the validity some general information about the transaction.</li>
        <li><!-- C -->The transaction is handed down to the executor's sub-executor designated for the transaction's type, which performs type-specific validation such as balance checks, address checks, etc.<br>&nbsp;</li>
    </ol>

12. The validation work performed by the calling node in step 11 is now repeated across the entire BVN by all of the calling node's peers. Tendermint calls CheckTx from each peer node. Steps 12- A, B, and C are identical to those of step 11.  
If a two-thirds majority of nodes agree that the transaction is valid, then Tendermint adds the transaction to a block of transactions maintained by Tendermint. Once a block is created, transactions can be added to it for a set length of time up to a set number of transactions. After the block is closed, Tendermint proceeds to step 13.

13. Tendermint notifies all consenting nodes in the BVN that each transaction in the block assembled in step 12 should now be executed. Nodes which failed step 12 but which were overruled by the two-thirds majority are ignored for the rest of this execution.  
A fork of the existing ledger is created and transactions are executed against this fork.  
Transactions are executed in order, one at a time, each one being executed on all the BVN's nodes simultaneously:

    <ol type='A'>
        <li><!-- A -->The transaction is sent to each node for execution.</li>
        <li><!-- B -->The transaction is handed down to the node's general executor for processing.</li>
        <li>
            <!-- C -->Another validity check is made for this transaction.  
            The transaction was valid in steps 11 and 12 when executed by itself against the main ledger, but in step 13 other transactions already executed from this block may have changed the state of the forked ledger such that this transaction is no longer valid. If that is the case, then execution stops and the entire block is scrapped.
        </li>
        <li>The transaction is committed to the forked ledger.</li>
    </ol>

14. Tendermint notifies <i>all</i> nodes in the BVN that the block was successful and that the forked ledger should now be permanently committed to the main ledger. Nodes which failed step 12 (and then sat out step 13) either apply this update or fall out of sync, ultimately resulting in their removal from Tendermint voting. TODO: verify


TODO: Append synthetic transaction trace, which begins at step 13.