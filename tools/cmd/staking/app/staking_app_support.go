package app

import "net/url"

// Deposit
// This structure tracks deposits to ensure that the tokens receive
// rewards only after the holding period as defined by the Staking App
type Deposit struct {
	MajorBlock int64 // The major block when the deposit was made
	Amount     int64 // Amount of tokens deposited into the Account
}
// UrlEntry
// Tracks when staking accounts are added to the protocol.
type UrlEntry struct {
	MajorBlock int64 // The major block when the URL was recorded
	Url        url.URL
}
// Account
// All tracking is done on a per account bases.  We could do more complex 
// tracking, but for now we will assume an outside process vets and approves
// all Staking Accounts
type Account struct {
	MajorBlock int64         // The Major Block where Account was approved
	URL        *url.URL      // The URL of this account
	DepositURL *url.URL      // The URL of the account to be paid rewards
	Entries    []interface{} // Returns some struct from the account
	Type       string        // Type of account
	Delegatee  *Account      // If this is a delegate, Account it delegates to.
	Balance    int64         // Balance if this is a token account
	Delegates  []*Account    // If this is a staker, and it has delegates
}