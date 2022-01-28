# Launch Detail

_This document describes how the various parts of this project actually execute. It's fine and dandy to say "oh a client sends a request to such-and-such a server which then does whatever" but that's not super useful if we don't know how a client or server comes into being as described by the code. This document is a guide to those processes._

There are essentially two different launch procedures: client and node.

<br>

---
---

<br>

## Client

A client is any program which can send correctly formatted binary data to a listening node. DeFiDevs provides a client within this project which accomplishes its task by using JSON RPC. Its launch is executed via the following trace:

<table>
    <thead>
        <tr>
            <th>Step</th>
            <th>Method</th>
            <th>...executed from file...</th>
            <th>...to accomplish:</th>
        </tr>
    </thead>
    <tbody>
        <tr>
            <td>1</td>
            <td><code>main</code></td>
            <td><code>/cmd/accumulate/main.go</code></td>
            <td>This is the entry point of execution from the parent OS.</td>
        </tr><tr>
            <td>2</td>
            <td><code>Execute</code></td>
            <td><code>/cmd/accumulate/cmd/root.go</code></td>
            <td>
                Uses COBRA to set up the root CLI command and adds subcommands for all client functions, such as querying the Accumulate network or sending tokens or whatever.
            </td>
        </tr>
    </tbody>
</table>

Subcommands are defined in files sibling to <code>/cmd/accumulate/cmd/root.go</code>. Dispatch of network-interfacing commands is defined in <code>/cmd/accumulate/cmd/util.go</code>. The destination(s) for RPC dispatches are defined by environment variable <code>ACC_API</code>.

**Keep in mind** that this is only the default, "stock" client provided by DeFiDevs. A client could be a CLI, a DApp backend, a mobile app, or the whims of some wizened lady using IPOAC from her coven in Nebraska.

<br>

---
---

<br>

## Node

A node is any instance of the Accumulate daemon. There are three primary properties of a node which define its function:

<table>
    <thead>
        <tr>
            <th colspan='2' style='text-align:center'>Property</th>
            <th>Effect</th>
        </tr>
    </thead>
    <tbody>
        <tr>
            <td rowspan='2' style='text-align:center'>Qualification</td>
            <td style='text-align:center'>Qualified</td>
            <td>The node operator has been vetted and has staked in the Accumulate network and maintains a near-100% uptime.<br>This node is eligible to vote in Tendermint consensus and may become the governor.</td>
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>Non-Qualified</td>
            <td>The node may participate fully in the Accumulate network except that it can never vote and will never become the governor.</td>
        </tr><tr>
            <td rowspan='2' style='text-align:center'>Network</td>
            <td style='text-align:center'>BVN</td>
            <td>The node handles routine transaction operations as a member of an arbitrary BVN.</td>
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>Directory</td>
            <td>The node participates in maintenance of the top levels of the blockchain and does not handle routine transactions.</td>
        </tr><tr>
            <td rowspan='2' style='text-align:center'>Size</td>
            <td style='text-align:center'>Full</td>
            <td>A full node which can validate and execute any kind of transaction as long as the transaction is routed to the right node.</td>
        </tr><tr>
            <!-- col -->
            <td style='text-align:center'>Partial</td>
            <td>A fragment of a Node, which validates and executes only some transactions. Other transactions are handled by a partial node's (also partial) peers. A partial node and all of its peers together make up a single full node (either qualified or non-qualified).</td>
        </tr>
    </tbody>
</table>

A node's qualification level is determined by the rest of the network. TODO: How, specifically? Its network type and its size are determined by configuration at startup.

Initialization of a node follows the following procedure:

<table>
    <thead>
        <tr>
            <th>Step</th>
            <th>Method</th>
            <th>...executed from file...</th>
            <th>...to accomplish:</th>
        </tr>
    </thead>
    <tbody>
        <tr>
            <td>1</td>
            <td><code>main</code></td>
            <td><code>/cmd/accumulated/main.go</code></td>
            <td>
                <ul>
                    <li>This is the entry point of execution from the parent OS.</li>
                    <li>
                        Uses COBRA to set up the root CLI command and adds subcommands for all node functions. One of these subcommands is <code>run</code> which launches the node instance.
                    </li>
                    <li>After COBRA setup, executes the <code>run</code> subcommand.
                </ul>
            </td>
        </tr><tr>
            <td>2</td>
            <td><code>runNode</code></td>
            <td><code>/cmd/accumulated/cmd_run.go</code></td>
            <td>Creates and launches the Accumulate daemon as a service on the host OS.</td>
        </tr><tr>
            <td>A</td>
            <td><code>Start</code></td>
            <td><code>/cmd/accumulated/program.go</code></td>
            <td>After calling necessary loading/config functions, starts the daemon.</td>
        </tr><tr>
            <td>B</td>
            <td><code>Start</code></td>
            <td><code>/internal/accumulated/run.go</code></td>
            <td>
                Performs setup and initialization of the daemon, including calling the launch of the Tendermint node and JRPC listener.
            </td>
        </tr><tr>
            <td>B.1</td>
            <td><code>New</code></td>
            <td><code>[ Tendermint black box ]</code></td>
            <td>Initializes a Tendermint node.</td>
        </tr><tr>
            <td>8.2.1</td>
            <td><code>NewJrpc</code></td>
            <td rowspan='2'><code>/internal/api/v2/jrpc.go</code></td>a
            <td rowspan='2'>
                Launches the JRPC listener, which effectively <i>is</i> the node or "server".
            </td>
        </tr><tr>
            <td>B.2.2</td>
            <td><code>NewMux</code></td>
        </tr>
    </tbody>
</table>