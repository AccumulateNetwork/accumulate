# Transaction Trace

_This document describes in detail the movement of a single transaction through the Accumulate system. For a less-detailed, more macroscopic and theoretical description of the system, see General_Architecture.md._

Transactions begin at the top of this table and move downward. More detailed descriptions of each step are available below.

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
            <td><code>[ The JSONRPC2 black box ]</code></td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <td rowspan='0'><b>R<br>E<br>C<br>E<br>I<br>V<br>E</td>
            <td style='text-align:center'>5</td>
            <td><code>ExecuteAddCredits</code></td>
            <td><code>/internal/api/v2/api_gen.go</code></td>
            <td rowspan='4'>A JSON RPC listener.</td>
            <td rowspan='4'>A node server running a BVN node instance.</td>
            <td rowspan='4'>A BVN.</td>
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>6</td>
            <td><code>executeWith</code></td>
            <td rowspan='3'>
                <code>/internal/api/v2/jrpc_execute.go</code><br>
                This file expands upon <code>./jrpc.go</code>
            </td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>7</td>
            <td><code>execute</code></td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>7 - A</td>
            <td><code>executeBatch</code></td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>7 - B</td>
            <td><code>CallBatch</code></td>
            <td><code>[ The JSONRPC2 black box ]</code></td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>7 - C</td>
            <td><code>Execute</code></td>
            <td rowspan='3'><code>/internal/api/v2/jrpc_execute.go</code></td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>7 - D</td>
            <td><code>execute</code><td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
            <!-- col -->
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
            <td>TODO: An unknown location.</td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <td colspan='200'><i>WIP...</td>
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

7. Routing instructions are consulted and the transaction is either sent to step 8 for local execution or sent to step 7-A to be redirected to another node.

    <ol type='A'>
        <li>If the transaction is not to be executed locally, then it is sent with a batch of outgoing transactions to the appropriate remote BVN.</li>
        <li>The batch is sent via the JSONRPC2 black box.</li>
        <li>Execution returns from the JSONRPC2 black box at <code>Execute</code>.
        <li>The same <code>execute</code> method from step 7 is repeated, but this time guaranteed to proceed to step 8 because this node is necessarily in the correct BVN for this transaction.<br>&nbsp;</li>
    </ol>  

8. The transaction is prepared for broadcast to Tendermint.

9. The transaction is sent to Tendermint.