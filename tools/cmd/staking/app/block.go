package app

import (
	"net/url"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Block struct {
	BlockHash         [32]byte   // Merkle Dag of the Major Block
	SetBudget         bool       // True if this block is the block to compute the weekly budget
	PrintReport       bool       // True if this block is the one to build the reward report
	PrintPayoutScript bool       // True if this block is the one to print out the Payout script
	MajorHeight       int64      // The major block number
	Timestamp         time.Time  // Timestamp of the DN defining the major block
	Accounts          []*Account // A list of the watched accounts that where modified in the major block
	Transactions      map[[32]byte][]*protocol.Transaction
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
