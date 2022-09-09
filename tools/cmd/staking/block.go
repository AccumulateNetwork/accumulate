package main

import (
	"net/url"
	"time"
)

type Block struct {
	MajorHeight int64
	MinorHeight int64
	Timestamp   time.Time
	Accounts    []*Account
}

// GetAccount
// Look through the accounts modified in this block, and return the
// account if it has been modified.
func (b *Block) GetAccount(AccountURL *url.URL) *Account {
	for _, acc := range b.Accounts {
		if acc.URL.String() == AccountURL.String() {
			return acc
		}
	}
	return nil
}

// GetTotalStaked
// Return the total number of tokens added to staking in this block
// The number can be negative.  Note Type == "" will return the total
// balance of all staking type accounts
func (b *Block) GetTotalStaked(Type string) (TotalTokens int64) {
	for _, acc := range b.Accounts {
		switch {
		case Type != "" && PureStaker == acc.Type: // If specified a type, then just add the type
			TotalTokens += acc.Balance

		case Type != "" && ProtocolValidator == acc.Type: // If specified a type, then just add the type
			TotalTokens += acc.Balance

		case Type != "" && ProtocolFollower == acc.Type: // If specified a type, then just add the type

		case Type != "" && StakingValidator == acc.Type: // If specified a type, then just add the type
			TotalTokens += acc.Balance

		case Type != "" && Delegate == acc.Type && Type == acc.Delegatee.Type: // If specified a type,
			TotalTokens += acc.Balance //                                         then add if right type

		case PureStaker == acc.Type, // if Not specifying a type, then add all staking types
			ProtocolValidator == acc.Type,
			ProtocolFollower == acc.Type,
			StakingValidator == acc.Type,
			Delegate == acc.Type:

			TotalTokens += acc.Balance
		}
	}
	return TotalTokens
}
