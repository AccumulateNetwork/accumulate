package refactor

// DCNode
// Create an instance of the Directory Chain Node.  Every node can run
// one of these.
type DCNode struct {
	State DCState
}

func (d DCNode) SendMessage(t *GenTransaction) {

}
