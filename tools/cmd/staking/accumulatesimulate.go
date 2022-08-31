package main


type Simulate struct {
	States     *State             // List of states per Major Block
	Registered []*StakingADI      // List of Registered Stakers
	Approved   []*StakingADI      // List of Approved Stakers
	Accounts   map[string]Account // Map of Staking Accounts
	Parameters Parameters         // The list of parameters for Staking Service
	App        *StakingApp        // This is the staking App
}

type State struct {
	Block      *Block        // The Major Block for this State
	Parameters *Parameters   // The current set of Parameters defining Staking
	Registered []*StakingADI // The set of new Registered StakingADI
	Approved   []*StakingADI // The set of new Approved StakingADI (but not yet Registered)
	Accounts   []*Account    // The set of new Staking Accounts
	Deposits   []*Deposit    // The set of new Deposits
}


func (s *Simulate) Start() {
	go func() { // Increment the major block and minor block
		
	}()

	for i := 0; i < 50; i++ {
		staker := new(StakingADI)
		staker.Activation = 14
		staker.AdiUrl, staker.AccountUrl = GenUrl("StakingAccount")
		switch i % 4 {
		case 0:
			staker.Type = PureStaker
		case 1:
			staker.Type = ProtocolValidator
		case 2:
			staker.Type = ProtocolFollower
		case 3:
			staker.Type = StakingValidator
		}
	}

}
