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

func (s *Simulator) StartOfMonth(block *Block) bool {
	p := s.GetParameters()
	offset := p.SetBudgetFreq - p.FirstSetBudget
	return offset == 7
}
