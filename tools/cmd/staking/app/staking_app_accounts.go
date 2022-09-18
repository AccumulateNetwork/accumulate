package app

import "sort"

// AddAccounts()
// Adds Approved accounts to the Staking Application state.
func (s *StakingApp) AddAccounts() {
	registered := s.CBlk.GetAccount(Registered)
	if registered == nil {
		return
	}

	for _, v := range registered.Entries {
		sa := v.(*Account)                                            // Get new registered account
		if oldSa, ok := s.Stakers.AllAccounts[sa.URL.String()]; !ok { // Is this a new account?
			s.Stakers.AllAccounts[sa.URL.String()] = sa //               Just add new accounts
		} else { //                                                      If an old account
			if oldSa.Type != sa.Type { //                                Check if its type has changed
				s.Stakers.AllAccounts[sa.URL.String()] = sa //           If type changed, replace
			} //                                                         Otherwise it isn't a change; ignore
		}
		switch sa.Type {
		case PureStaker:
			s.Stakers.Pure = append(s.Stakers.Pure, sa)
		case ProtocolValidator:
			s.Stakers.PValidator = append(s.Stakers.PValidator, sa)
		case ProtocolFollower:
			s.Stakers.PFollower = append(s.Stakers.PFollower, sa)
		case StakingValidator:
			s.Stakers.SValidator = append(s.Stakers.SValidator, sa)
		case Delegate:
			sa.Delegatee.Delegates = append(sa.Delegatee.Delegates, sa)
		}
	}
	sa := func(a []*Account) []*Account {
		sort.Slice(a, func(i, j int) bool { return a[i].URL.String() < a[j].URL.String() })
		for _, a2 := range a {
			d := a2.Delegates
			sort.Slice(d, func(i, j int) bool { return d[i].URL.String() < d[j].URL.String() })
		}
		return a
	}
	sa(s.Stakers.Pure)
	sa(s.Stakers.PValidator)
	sa(s.Stakers.PFollower)
	sa(s.Stakers.SValidator)
}