package main

func (s *Simulator) GetParameters() *Parameters {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.parameters
}

func (s *Simulator) GetBlock(idx int64) *Block {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if idx < 0 || idx >= int64(len(s.MajorBlocks)){
		return nil
	}
	blk := s.MajorBlocks[idx]
	d := blk.MajorHeight - s.parameters.PayOutFirst
	blk.PrintReport = d > 0 && d%s.parameters.PayOutFreq == 0
	blk.PrintPayoutScript = d > 0 && d%s.parameters.PayOutFreq == 1
	d = blk.MajorHeight - s.parameters.SetBudgetFirst
	blk.SetBudget = d > 0 && d%s.parameters.SetBudgetFreq == 0
	return blk
}

func (s *Simulator) GetTokensIssued() int64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.TokensIssued
}

func (s *Simulator) StartOfMonth(block *Block) bool {
	p := s.GetParameters()
	offset := p.SetBudgetFreq - p.SetBudgetFirst
	return offset == 7
}

func (s *Simulator) IssuedTokens(tokens int64) {
	s.TokensIssued-=tokens
}