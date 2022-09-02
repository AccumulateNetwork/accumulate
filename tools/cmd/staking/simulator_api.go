package main

func (s *Simulator) GetParameters() *Parameters {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.parameters
}

func (s *Simulator) GetBlock() *Block {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.CurrentBlock
}

func (s *Simulator) GetTokensIssued() int64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.TokensIssued
}
