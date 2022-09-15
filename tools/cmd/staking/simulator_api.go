package main

func (s *Simulator) GetParameters() *Parameters {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.parameters
}

// GetBlock
// Get a Block by its index.  Note that Blocks MUST be retrieved in order.
// That's okay, because to build up the state within the staking app, all 
// major blocks must be read and processed.  This means to update the 
// Staking App for a year, we need to access 730 or so blocks (365*2). 
// Not that heavy of a lift.
func (s *Simulator) GetBlock(idx int64) *Block {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if idx < 0 || idx >= int64(len(s.MajorBlocks)){
		return nil
	}
	blk := s.MajorBlocks[idx]
	if idx == 0 {
		blk.SetBudget = true
		blk.PrintReport = false
		blk.PrintPayoutScript = false
		return blk
	}
	day := blk.Timestamp.UTC().Day()
	hour := blk.Timestamp.UTC().Hour()
	if blk.MajorHeight>1 && day == 1 && hour==0 { // The month changed
		blk.SetBudget = true
	}
	// The payday starts when the last block on Thursday completes.
	if idx > 13 && blk.Timestamp.UTC().Weekday() == 5 && blk.Timestamp.UTC().Hour()==0 {
		blk.PrintReport = true
	}
	// The script is printed in the major block after the report is produced
	if idx > 13 && s.MajorBlocks[idx-1].PrintReport{
		blk.PrintPayoutScript = true
	}
	return blk
}

func (s *Simulator) GetTokensIssued() int64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.TokensIssued
}

func (s *Simulator) IssuedTokens(tokens int64) {
	s.TokensIssued-=tokens
}