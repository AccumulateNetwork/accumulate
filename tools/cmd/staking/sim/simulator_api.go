package sim

import "gitlab.com/accumulatenetwork/accumulate/tools/cmd/staking/app"

func (s *Simulator) GetParameters() (*app.Parameters, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.parameters, nil
}

// GetBlock
// Get a Block by its index.  Note that Blocks MUST be retrieved in order.
// That's okay, because to build up the state within the staking app, all
// major blocks must be read and processed.  This means to update the
// Staking App for a year, we need to access 730 or so blocks (365*2).
// Not that heavy of a lift.
// Code assumes the accountData map is not accessed by the caller after
// calling GetBlock()
func (s *Simulator) GetBlock(idx int64, accounts map[string]int) (*app.Block,error){
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Note that this works because we query blocks far more often than blocks are ready
	//   and that means any changes in the accounts to query made in one block will get
	//   updated in the Simulator before the next block is produced.
	s.AccountData = accounts // Caller builds a new map with every call.

	if idx < 0 || idx >= int64(len(s.MajorBlocks)) {
		return nil, nil
	}
	return s.MajorBlocks[idx],nil

}


func (s *Simulator) GetTokensIssued() (int64, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.tokensIssued, nil
}

func (s *Simulator) IssuedTokens(tokens int64) {
	s.tokensIssued -= tokens
}
