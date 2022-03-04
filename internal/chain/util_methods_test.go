package chain

func (e *Executor) ForceCommit() ([]byte, error) {
	return e.commit(true)
}
