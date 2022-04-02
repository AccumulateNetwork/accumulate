package block

// Whitebox testing utilities

// Ping pings the governor. If runDidCommit is running, Ping will block until it
// completes.
func (g *governor) Ping() {
	select {
	case g.messages <- govPing{}:
	case <-g.done:
	}
}

// WaitForGovernor pings the governor, waiting until runDidCommit completes.
func (x *Executor) WaitForGovernor() {
	x.governor.Ping()
}
