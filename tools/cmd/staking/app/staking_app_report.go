package app

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path"
	"sort"

	"go.uber.org/zap/buffer"
)

// TotalAccounts
// Get the total tokens from a set of Accounts.  Returned as:
//   - sum (just the account balances), and
//   - sumD (account balances + delegate balances)
func TotalAccounts(Account []*Account) (sum, sumD int64) {
	for _, a := range Account {
		sum += a.Balance
		sumD += a.Balance
		for _, d := range a.Delegates {
			sumD += d.Balance
		}
	}
	return sum, sumD
}

// Report
// Prints out a tab delimitated excel spreadsheet.  This spreadsheet
// does most of the calculations used to distribute tokens.  It is easily
// reviewed by users by loading the report into Excel or Google Sheets or
// other spreadsheet.
func (s *StakingApp) Report() {

	// First check if CBlk is a payout block.  If not, return
	if !s.CBlk.PrintReport && !s.CBlk.PrintPayoutScript {
		return
	}

	s.Stakers.Distributions = s.Stakers.Distributions[:0]      // Clear out past distributions, start fresh
	s.Stakers.DistributionMap = make(map[string]*Distribution) // Make a cleared Distribution Map

	unissued := 500000000 - s.Data.TokensIssued
	rewards := unissued / 16 / 100
	s.protocol.TokensIssued(rewards)

	s.Collect() // Collect all the needed information for the payout

	if s.CBlk.PrintReport {
		s.Log(fmt.Sprintf("Writing Report: %d ", s.CBlk.MajorHeight))
	} else {
		s.Log(fmt.Sprintf("Writing Payout Script: %d ", s.CBlk.MajorHeight))

	}

	report := new(buffer.Buffer)
	f := func(format string, a ...any) {
		report.WriteString(fmt.Sprintf(format, a...))
	}
	f("Report:\tMajor Block Height\t%d\n", s.Data.BlockHeight)                                              // 1
	f("\tMajorBlockTimestamp\t%s\n", s.Data.Timestamp)                                                      // 2
	f("\n")                                                                                                 // 3
	f("\tBudget\n")                                                                                         // 4
	f("\tACME Token limit\t%d\n", s.Data.TokenLimit)                                                        // 5
	f("\tACME Tokens Issued\t%d\n", s.Data.TokensIssued)                                                    // 6
	f("\tToken Issuance Rate\t%d%%\n", s.Data.TokenIssuanceRate)                                            // 7
	f("\tAnnual Budget\t=round((C5-C6)*C7,4)\n")                                                            // 8
	f("\tWeekly Budget\t=round(C8/365.25*7,4)\n")                                                           // 9
	f("\tAPR\t=round(c8/c21,4)\n")                                                                          // 10
	f("\t\n")                                                                                               // 11
	f("\tWeight Pure Staking Account\t=round(%d%%,4)\n", s.Data.WtPS)                                       // 12
	f("\tWeight Protocol Validator\t=round(%d%%,4)\n", s.Data.WtPV)                                         // 13
	f("\tWeight Protocol Follower\t=round(%d%%,4)\n", s.Data.WtPF)                                          // 14
	f("\tWeight Staking Validator\t=round(%d%%,4)\n", s.Data.WtSV)                                          // 15
	f("\n")                                                                                                 // 16
	f("\tDelegated Account Return\t=round(%d%%,4)\n", s.Data.DelegatedAccountReturn)                        // 17
	f("\tStaking Share of Delegated Tokens\t=round(%d%%,4)\n", s.Data.DelegateeShare)                       // 18
	f("\n")                                                                                                 // 19
	f("\t\tPure Tokens\tWeighted Tokens\tReward\n")                                                         // 20
	f("\tTotal Staked Tokens\t=Round(%d,4)\t=Round(%d,4)\t=C9\n", s.Data.TotalTokens, s.Data.TotalWeighted) // 21
	fm := "\tTotal %s Accounts\t%d\t%d\t=round(C$9/D$21*%s,4)\n"                                            //
	m := func(tokens int64, wt float64) int64 { return int64(float64(tokens) * wt / 100) }                  //
	f(fm, "Pure Staking", s.Data.TotalPSD, m(s.Data.TotalPSD, float64(s.Data.WtPS)), "D22")                 // 22
	f(fm, "Protocol Validator", s.Data.TotalPVD, m(s.Data.TotalPVD, float64(s.Data.WtPV)), "D23")           // 23
	f(fm, "Protocol Follower", s.Data.TotalPFD, m(s.Data.TotalPFD, float64(s.Data.WtPF)), "D24")            // 24
	f(fm, "Staking Validator", s.Data.TotalSVD, m(s.Data.TotalSVD, float64(s.Data.WtSV)), "D25")            // 25
	f("\n")                                                                                                 // 26
	f("\tTotal Weighted Staked Tokens\t=D21\n")                                                             // 27
	f("\tReward per Pure Staking token\t=round(E22/C22,8)\n")                                               // 28
	f("\tReward per Protocol Validator Token\t=round(E23/C23,8)\n")                                         // 29
	f("\tReward per Protocol Follower Token\t=round(E24/C24,8)\n")                                          // 30
	f("\tReward per Staking Validator Token\t=round(E25/C25,8)\n")                                          // 31
	f("\t\n")                                                                                               // 32
	f("\tAPR Pure Staking\t=round(E22*(365.25/7)/c22,4)\n")                                                 // 33
	f("\tAPR Protocol Validator Staking\t=round(E23*(365.25/7)/c23,4)\n")                                   // 34
	f("\tAPR Protocol Follower Staking\t=round(E24*(365.25/7)/c24,4)\n")                                    // 35
	f("\tAPR Staking Validator Staking\t=round(E25*(365.25/7)/c25,4)\n")                                    // 36
	f("\n")                                                                                                 // 37
	f("\n")                                                                                                 // 38

	lines, end := s.PrintAccounts(PureStaker, "Pure Staking Accounts", s.Stakers.Pure, "c12", "c28", 39)
	f(lines)
	lines, end = s.PrintAccounts(ProtocolValidator, "Protocol Validator Accounts", s.Stakers.PValidator, "c13", "c29", end)
	f(lines)
	lines, end = s.PrintAccounts(ProtocolFollower, "Protocol Follower Accounts", s.Stakers.PFollower, "c14", "c30", end)
	f(lines)
	lines, end = s.PrintAccounts(StakingValidator, "Staking Validator Accounts", s.Stakers.SValidator, "c15", "c31", end)
	f(lines)

	lines, end = s.PrintDistributions(end)
	f(lines)
	lines, end = s.PrintDistributionScript(end)
	f(lines)

	_ = end //gets rid of a annoying warning. All subprocesses that output to the spreadsheet return end.

	reportFile := path.Join(ReportDirectory, fmt.Sprintf("Report-%d.csv", s.CBlk.MajorHeight))
	if s.CBlk.PrintPayoutScript {
		reportFile = path.Join(ReportDirectory, fmt.Sprintf("Payout-%d.csv", s.CBlk.MajorHeight))
	}

	if f, err := os.Create(reportFile); err != nil {
		panic(fmt.Sprintf("Could not create %s: %v", reportFile, err))
	} else {
		if _, err := f.Write(report.Bytes()); err != nil {
			panic(fmt.Sprintf("could not write to file %s: %v", reportFile, err))
		}
		if err := f.Close(); err != nil {
			panic(fmt.Sprintf("could not close file %s: %v", reportFile, err))
		}
	}
}

// PrintAccounts
// Writes out all the lines staking token accounts (the staking account and any delegates).
func (s *StakingApp) PrintAccounts(
	Type string,
	Label string,
	accounts []*Account,
	weight, reward string,
	start int) (lines string, end int) {

	var report buffer.Buffer
	end = start
	f := func(format string, a ...any) {
		report.WriteString(fmt.Sprintf(format, a...))
		end++
	}

	f("------------------------------------------------------------------------------------------------\n")
	f("\t%s\n", Label)
	f("\tURL\tAccount Tokens\tTotal Tokens\tWeighted Tokens\tTotal Reward\tAccount Reward\tDeposit URL\n")
	for _, a := range accounts {
		if a.Type == Type && a.MajorBlock < s.CBlk.MajorHeight {

			// RecordDistribution
			// A function in one place, that needs to be done in 3 cases
			RecordDistribution := func(a *Account) {
				d := new(Distribution)                                       // Create the distribution record
				d.Account = a                                                // Type of Distribution
				d.Tokens = end - 1                                           // Calculate the Distribution AFTER printing
				s.Stakers.Distributions = append(s.Stakers.Distributions, d) // Add to distribution list
			}
			RecordDelegateDistribution := func(a *Account) {
				d := new(Distribution)                        // Create a Distribution for Delegates
				d.Account = a                                 // Put Delegate accounts in map (sorted by the Delegatee)
				d.Tokens = end - 1                            // Calculate the Distribution AFTER printing
				s.Stakers.DistributionMap[a.URL.String()] = d // Add delegates to the Distribution map
			}
			dl := len(a.Delegates) // Calculate the number of lines for delegates
			// URL | BALANCE | =Balance+sum | (Balance+sum)*(WEIGHT) | Total REWARD | Account Reward | Deposit URL
			if len(a.Delegates) > 0 {
				fm := "\t%s" +
					"\t=round(%d,4)" +
					"\t=round(sum(C%d:C%d),4)" +
					"\t=round(D%d*%s,4)" +
					"\t=round(E%d*%s,4)" +
					"\t=round(C%d*%s+sum(C%d:C%d)*%s*%s*C18,4)" +
					"\t%s\n"
				f(fm,
					a.URL.String(),
					a.Balance,
					end, end+dl,
					end, weight,
					end, reward,
					end, reward, end+1, end+dl, reward, weight,
					a.DepositURL.String())

				RecordDistribution(a)

				fd := "Delegate" +
					"\t%s" +
					"\t=round(%d,4)" +
					"\t\t\t" +
					"\t=round(C%d*%s*%s*C17,4)\n"
				for _, d := range a.Delegates {
					f(fd, d.URL.String(), d.Balance, end, weight, reward)
					RecordDelegateDistribution(d)
				}

			} else {
				fm := "\t%s" +
					"\t=round(%d,4)" +
					"\t=round(sum(C%d:C%d),4)" +
					"\t=round(D%d*%s,4)" +
					"\t=round(E%d*%s,4)" +
					"\t=round(C%d*%s,4)" +
					"\t%s\n"
				f(fm,
					a.URL.String(),
					a.Balance,
					end, end+dl,
					end, weight,
					end, reward,
					end, reward,
					a.DepositURL.String())

				RecordDistribution(a)

			}
		}
	}
	return report.String(), end
}

// GetMasterOrder
// Return the List of Staking Validators in the order of their responsibility to submit
// the distribution transaction(s) for staking
func (s *StakingApp) GetMasterOrder() []*Account {
	// Make sure the Staking validator URLs are in order.  This ensures all validators are 
	// going to mix the same list starting from the same starting state.
	sort.Slice(s.Stakers.SValidator, func(i, j int) bool {
		return s.Stakers.SValidator[i].URL.String() < s.Stakers.SValidator[j].URL.String()
	})
	
	// Our mix is slower than it has to be.  We are going to give each element in the list
	// a hash derived from and starting with the block hash.
	MasterList := append([]*Account{}, s.Stakers.SValidator...) // create a master list
	h := s.CBlk.BlockHash                                       // Use the block hash as the basis of randomizing
	for _,a:=range MasterList { // Give every element an Order hash based on the BlockHash
		h = sha256.Sum256(h[:]) 
		a.Order = h
	}
	sort.Slice(MasterList,func(i,j int)bool{ // Sort the master list by its Order Hash
		return bytes.Compare(MasterList[i].Order[:],MasterList[j].Order[:])==-1	
	})
	return MasterList
}

func (s *StakingApp) PrintDistributions(start int) (lines string, end int) {
	var report buffer.Buffer
	end = start
	f := func(format string, a ...any) {
		report.WriteString(fmt.Sprintf(format, a...))
		report.WriteByte('\n')
		end++
	}

	f("\n\n+===========================================================================================================")
	f("+================================= Distribution Summary ====================================================")
	f("+===========================================================================================================")

	sort.SliceStable(s.Stakers.Distributions, func(i, j int) bool {
		return s.Stakers.Distributions[i].Account.URL.String() < s.Stakers.Distributions[j].Account.URL.String()
	})
	sort.SliceStable(s.Stakers.Distributions, func(i, j int) bool {
		return s.Stakers.Distributions[i].Account.Type < s.Stakers.Distributions[j].Account.Type
	})

	var t string
	for _, d := range s.Stakers.Distributions {
		if d.Account.Type != t {
			f("\n------------------------------------------------------------------------------------")
			f(d.Account.Type)
			t = d.Account.Type
		}
		f("\t%s\t\t%s\t=G%d", d.Account.URL.String(), d.Account.DepositURL, d.Tokens)
		sort.Slice(d.Account.Delegates, func(i, j int) bool {
			return d.Account.Delegates[i].URL.String() < d.Account.Delegates[j].URL.String()
		})
		for _, del := range d.Account.Delegates {
			dist := s.Stakers.DistributionMap[del.URL.String()]
			f("\t\t%s\t%s\t=G%d", del.URL.String(), del.DepositURL, dist.Tokens)
		}
	}

	return report.String(), end
}

func (s *StakingApp) PrintDistributionScript(start int) (lines string, end int) {
	if !s.CBlk.PrintPayoutScript {
		return "", start
	}
	var report buffer.Buffer
	end = start
	f := func(format string, a ...any) {
		report.WriteString(fmt.Sprintf(format, a...))
		report.WriteByte('\n')
		end++
	}
	// accumulate token issue [adi token url] [signer key name] [recipient url] [amount] [flags]
	f("\n\n+================================================================================================================")
	f("+================================= Token Distribution Script ====================================================")
	f("+================================================================================================================")
	sort.SliceStable(s.Stakers.Distributions, func(i, j int) bool {
		return s.Stakers.Distributions[i].Account.URL.String() < s.Stakers.Distributions[j].Account.URL.String()
	})
	sort.SliceStable(s.Stakers.Distributions, func(i, j int) bool {
		return s.Stakers.Distributions[i].Account.Type < s.Stakers.Distributions[j].Account.Type
	})

	for _, d := range s.Stakers.Distributions {

		f("accumulate token issue acc://acme \tValidatorKey \t%s\t=G%d", d.Account.DepositURL, d.Tokens)
		for _, del := range d.Account.Delegates {
			dist := s.Stakers.DistributionMap[del.URL.String()]
			f("accumulate token issue acc://acme \tValidatorKey \t%s\t=G%d", del.DepositURL, dist.Tokens)
		}
	}

	return report.String(), end
}
