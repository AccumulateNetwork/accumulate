package network

// ProtocolState
// Defines the parameters and structure of the protocol state.  This state is
// kept in sync with all BVCs and the DVC.
type ProtocolState struct {
	DVC  *DVCState     // Every node must keep up with the DVC, but not all nodes have to track a BVC
	BVC  *BVCState     // If we are tracking a BVCState, then this pointer is not nil
	BVCs []*BVCNetwork // Pointer to the communication network for the BVCs
}
