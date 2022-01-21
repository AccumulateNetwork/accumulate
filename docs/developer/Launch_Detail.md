# Launch Detail

_This document describes how the various parts of this project actually execute. It's fine and dandy to say "oh a client sends a request to such-and-such a server which then does whatever" but that's not super useful if we don't know how a client or server comes into being as described by the code. This document is a guide to those processes._

There are essentially two different launch procedures: client and node.

---

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

---

## Node

A node is any program running the Accumulate daemon. Nodes come in the following flavors, each with different functions:

| Node Type | Function |
|---|---|
| Directory | TODO: Does some stuff? |
| Validator | TODO: Does some completely different stuff? |

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
                        Uses COBRA to set up the root CLI command and adds subcommands for all node functions.
                    </li>
                </ul>
            </td>
        </tr><tr>
            <td>2</td>
            <td colspan='200'><i>TODO: Intermediate steps unclear</i></td>
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
            <td>B.2</td>
            <td><code>NewMux</code></td>
            <td><code>/internal/api/v2/jrpc.go</code></td>
            <td>
                Launches the JRPC listener, which effectively <i>is</i> the node or "server".
            </td>
        </tr>
    </tbody>
</table>