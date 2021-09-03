package network

// SimNet
// When simulating a protocol, the SimNet manages all the nodes within the
// simulation and allows routing of transactions between nodes just as
// Tendermint would do.
type SimNet struct {
}

var _ BVCNetwork = (*SimNet)(nil)
