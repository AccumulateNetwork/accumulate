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
            <td>1</td>
            <td><code>AddCredits</code></td>
            <td><code>/cmd/accumulate/cmd/credits.go</code></td>
            <td rowspan='4'>An arbitrary client program.</td>
            <td rowspan='4'>A client device: a CLI, or a DApp backend, etc.</td>
            <td rowspan='4'>The user.</td>
        </tr><tr>
            <!-- col -->
            <td>2</td>
            <td><code>dispatchTXRequest</code></td>
            <td><code>/cmd/accumulate/cmd/util.go</code></td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td>3</td>
            <td><code>Request</code></td>
            <td><code>/client/client.go</code></td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td>4</td>
            <td><code>Request</code></td>
            <td><code>[ The JSONRPC2 black box ]</code></td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <td rowspan='0'><b>R<br>E<br>C<br>E<br>I<br>V<br>E</td>
            <td colspan='20'><i>TODO: Not sure how the next step gets called.</i></td>
        </tr><tr>
            <!-- col -->
            <td>?</td>
            <td><code>ExecuteAddCredits</code></td>
            <td><code>/internal/api/v2/api_gen.go</code></td>
            <td rowspan='4'>JSON RPC</td>
            <td rowspan='4'>A node</td>
            <td rowspan='4'>TODO: A BVN?</td>
        </tr><tr>
            <!-- col -->
            <td>?</td>
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
            <td>?</td>
            <td><code>execute</code></td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td>2</td>
            <td><code>executeBatch</code></td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <!-- col -->
            <td>3</td>
            <td><code>CallBatch</code></td>
            <td><code>[ The JSONRPC2 black box ]</code></td>
            <!-- col -->
            <!-- col -->
            <!-- col -->
        </tr><tr>
            <td colspan='200'><i>WIP...</td>
        </tr>
    </tbody>
</table>

1. In the CLI client implementation provided by DeFiDevs, this method is triggered by a user-entered command.
2. All of the specifics of the transaction request were handled in step 1. Now they are being packed up for sending.
3. The request is handed off to the JSONRPC client.
4. The request is processed by the JSONRPC client and actually sent.