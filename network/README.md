# Network
The network package models the Accumulate Protocol, which allows for a 
number of things:

* Simulation of an entire Accumulate deployment without running a number of 
  independent servers.
* Faster development and debugging of chain validators
* Facilitates consensus of the Network topology across the BVCs and the DC
* Allows unit testing of routing between Block Validator Chains (BVCs) and the 
  Directory Validator Chain (DVC) in unit tests
* Separates the consensus integration (Tendermint) from blockchain 
  construction (most of the Accumulate code)

## Tracking the topology of the protocol
Enumerates how many BVCs we are supporting
For each BVC, a network connection is defined.  Two network connections are 
possible:
1. A Tendermint connection to a BVC blockchain.  Note that each BVC 
   blockchain runs as a separate "side-chain" to all the other BVC 
   blockchains in the protocol.
2. A Simulated Network. A simulated network is made up of a set of 
   go-routines that stands in for the validator or follower nodes that 
   would be present on a full Accumulate Network.
    1. A Simulated Network runs a set of validators for each BVC.
    
## Structures and Interfaces
1. ProtocolState struct -- details the state information that all nodes in the 
   system must agree upon for the protocol to operate.
   - Holds the list of BVCNetworks
   - SubmitTransaction(transaction)
      - Routes transactions to the appropriate BVCNetwork
2. BVCNetwork Interface -- Provides for multiple implementations for 
   interacting with BVCs.  We anticipate two BVCNetwork choices:
   1. TenderNet struct -- Interface implementation allowing BVC servers to run 
      on their own Tendermint networks and coordinate between each other via 
      Tendermint. 
   2. SimNet struct --  Interface implementation allowing BVC servers to run 
      in their own go routines and coordinate between each other using go 
      channels.  Allows quick booting of a complete Accumulate network on a 
      single computer.
      - 
         
3. BVCState struct -- state information that a BVC node requires to operate 
   as a particular BVC node in the Accumulate Protocol
4. DVCState struct -- state information that a Directory Validator Chain (DVC) 
   node requires to operate as a particular DVC node in the Accumulate Protocol
5. DSState struct -- state information that a Data Server (DS) node requires 
   to operate as a Data Server in the Accumulate Protocol.   
   
