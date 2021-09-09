Refactor of the structure and flow of Accumulate

The following structures and Interfaces are needed:

    Either:
 - Instance
   
    This is the number of nodes that this Accumulate Instance is running.  
   In order to have the greatest capacity, the NodeList would run only one 
   node.  But in less taxed systems, multiple nodes can be run in a single 
   instance.  Holds (possibly) a DC node and a list of BVC nodes.
   
    - BVC node handles all the duties of a BVC node.  Can be a follower BVC or 
      participate as a BVC validator.  The NodeList can have many BVC nodes
    - DC node handles the duties of a DC node.  Can be a follower or 
      participate as a DC validator.  The NodeList can have 0 or 1 DC node.
      
    Each node manages:

    - DCState -- provides information about the entire state of the protocol
  
      Note that this state must be kept in sync across all DC and BVCs in 
      Accumulate
      
        The DCState contains a DCNetwork struct that manages coordinating 
      and possibly participating in the DirectoryChain operation

    - list of BVCNetwork Interfaces.
    
    Each BVCNetwork provides the interfaces used to communicate either 
    to Tendermint, or to a local simulated network of BVC nodes
        
    - Tendermint Struct -- used to connect to and communicate with a 
    Tendermint Network.
    - SimNetwork Struct -- used to connect to and communicate with a set of 
      nodes simulating a Network
    - <network struct> -- could be used to test other consensus implemenations
    
    
    