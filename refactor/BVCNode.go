package refactor

// BVCNode
// Instance of the Directory Chain Node.
type BVCNode struct {
	State DCState // Defines the state of the Accumulate Network
	Index int     // The index of the BVC network that this BVCNode belongs to
}

func (b BVCNode) SendMessage(t *GenTransaction) {

}
