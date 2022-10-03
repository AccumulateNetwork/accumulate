package app

import (
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/staking"
)

// AddAccounts()
// Adds Approved accounts to the Staking Application state.
func (s *StakingApp) AddAccounts() {
	registered := s.CBlk.GetAccount(Registered)
	if registered == nil {
		return
	}

	if oldSa, ok := s.Stakers.AllAccounts[registered.URL.String()]; !ok { // Is this a new account?
		s.Stakers.AllAccounts[registered.URL.String()] = registered //               Just add new accounts
	} else { //                                                      If an old account
		if oldSa.Type != registered.Type { //                                Check if its type has changed
			s.Stakers.AllAccounts[registered.URL.String()] = registered //           If type changed, replace
		} //                                                         Otherwise it isn't a change; ignore
	}
	switch registered.Type {
	case staking.AccountTypePure:
		s.Stakers.Pure = append(s.Stakers.Pure, registered)
	case staking.AccountTypeCoreValidator:
		s.Stakers.PValidator = append(s.Stakers.PValidator, registered)
	case staking.AccountTypeCoreFollower:
		s.Stakers.PFollower = append(s.Stakers.PFollower, registered)
	case staking.AccountTypeStakingValidator:
		s.Stakers.SValidator = append(s.Stakers.SValidator, registered)
	case staking.AccountTypeDelegated:
		registered.Delegatee.Delegates = append(registered.Delegatee.Delegates, registered)
	}

	// Function to sort an account
	sa := func(a []*Account) []*Account {
		sort.Slice(a, func(i, j int) bool { return a[i].URL.String() < a[j].URL.String() })
		for _, a2 := range a {
			d := a2.Delegates
			sort.Slice(d, func(i, j int) bool { return d[i].URL.String() < d[j].URL.String() })
		}
		return a
	}

	// Sort all the types of staker accounts
	sa(s.Stakers.Pure)
	sa(s.Stakers.PValidator)
	sa(s.Stakers.PFollower)
	sa(s.Stakers.SValidator)
}
