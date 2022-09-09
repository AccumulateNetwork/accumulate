package main

import (
	"fmt"
	"os"
	"path"
	"sort"
	"time"

	"go.uber.org/zap/buffer"
)

type StakingApp struct {
	Params *Parameters
	CBlk   *Block
	Data   struct {
		BlockHeight            int64
		Timestamp              string
		TokenLimit             int64
		TokensIssued           int64
		TokenIssuanceRate      int64
		WtPS                   int64
		WtPV                   int64
		WtPF                   int64
		WtSV                   int64
		DelegatedAccountReturn int64
		DelegateeShare         int64
		TotalTokens            int64 // Total staked tokens
		TotalWeighted          int64 // Total weighted tokens
		TotalPS                int64 // Total Pure Staking Tokens
		TotalPV                int64 // Total Protocol Validator Tokens
		TotalPF                int64 // Total Protocol Follower Tokens
		TotalSV                int64 // Total Staking Validator Tokens
		TotalPSD               int64 // Total Pure Staking Tokens + delegate Tokens
		TotalPVD               int64 // Total Protocol Validator Tokens + delegate Tokens
		TotalPFD               int64 // Total Protocol Follower Tokens + delegate Tokens
		TotalSVD               int64 // Total Staking Validator Tokens + delegate Tokens
		PSS                    int64 // Pure Staking Start
		PSE                    int64 // Pure Staking End
		PVS                    int64 // Protocol Validator Start
		PVE                    int64 // Protocol Validator End
		PFS                    int64 // Protocol Follower Start
		PFE                    int64 // Protocol Follower End
		SVS                    int64 // Staking Validator Start
		SVE                    int64 // Staking Validator End
	}
	sim     *Simulator
	Stakers struct {
		Registered map[string]*Registration // Registered ADIs (Registered ADIs can add new staking accounts)
		Delegate   map[string]*Account      // Maps Delegates to Accounts and prevents double payouts
		Pure       []*Account               // Pure Stakers
		PValidator []*Account               // Protocol Validators
		PFollower  []*Account               // Protocol Followers
		SValidator []*Account               // Staking Validators
	}
}

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

func (s *StakingApp) Run() {
	fmt.Println("Starting")
	sim := new(Simulator)
	s.sim = sim
	sim.Init()
	go sim.Run()
	s.Params = sim.GetParameters()
	s.CBlk = sim.GetBlock()
	for {
		b := sim.GetBlock()
		if b.MajorHeight == s.CBlk.MajorHeight {
			time.Sleep(time.Second / 4)
			continue
		}
		s.AddAccounts()
		s.CBlk = b // This is the new current block
		s.AddApproved(b)
		s.Report()
		time.Sleep(time.Second / 4)
	}
}

func (s *StakingApp) AddAccounts() {
	registered := s.CBlk.GetAccount(Registered)
	if registered == nil {
		return
	}
	newAccounts := make(map[string]*Account)
	for _, v := range registered.Entries {
		sa := v.(*Account)
		newAccounts[sa.URL.String()] = sa
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
	for _, v := range registered.Entries {
		sa := v.(*Account)
		for _, da := range sa.Delegates {
			_ = da
		}
	}
}

func (s *StakingApp) Collect() {
	s.Data.BlockHeight = s.CBlk.MajorHeight
	s.Data.Timestamp = s.CBlk.Timestamp.UTC().Format(time.UnixDate)
	s.Data.TokenLimit = s.Params.TokenLimit
	s.Data.TokensIssued = s.sim.GetTokensIssued()
	s.Data.TokenIssuanceRate = int64(s.Params.TokenIssuanceRate * 100)
	s.Data.WtPS = int64(s.Params.StakingWeight.PureStaking * 100)
	s.Data.WtPV = int64(s.Params.StakingWeight.ProtocolValidator * 100)
	s.Data.WtPF = int64(s.Params.StakingWeight.ProtocolFollower * 100)
	s.Data.WtSV = int64(s.Params.StakingWeight.StakingValidator * 100)
	s.Data.DelegatedAccountReturn = int64(s.Params.DelegateShare * 100)
	s.Data.DelegateeShare = int64(s.Params.DelegateeShare * 100)

	s.Data.TotalPS, s.Data.TotalPSD = TotalAccounts(s.Stakers.Pure)
	s.Data.TotalPV, s.Data.TotalPVD = TotalAccounts(s.Stakers.PValidator)
	s.Data.TotalPF, s.Data.TotalPFD = TotalAccounts(s.Stakers.PFollower)
	s.Data.TotalSV, s.Data.TotalSVD = TotalAccounts(s.Stakers.SValidator)
	s.Data.TotalTokens = s.Data.TotalPS + s.Data.TotalPV + s.Data.TotalPF + s.Data.TotalSV
	s.Data.TotalWeighted = int64(float64(s.Data.TotalPSD)*s.Params.StakingWeight.PureStaking +
		float64(s.Data.TotalPVD)*s.Params.StakingWeight.ProtocolValidator +
		float64(s.Data.TotalPFD)*s.Params.StakingWeight.ProtocolFollower +
		float64(s.Data.TotalSVD)*s.Params.StakingWeight.StakingValidator)
}

func (s *StakingApp) Report() {

	// First check if CBlk is a payout block.  If not, return
	if c := (s.CBlk.MajorHeight - s.Params.FirstPayOut) % s.Params.PayOutFreq; c != 0 {
		return
	}

	s.Collect() // Collect all the needed information for the payout

	fmt.Print("\nWriting that Report for block\n\t.\n")
	report := new(buffer.Buffer)
	f := func(format string, a ...any) {
		report.WriteString(fmt.Sprintf(format, a...))
	}
	f("Report:\tMajor Block Height\t%d\n", s.Data.BlockHeight)                                    // 1
	f("\tMajorBlockTimestamp\t%s\n", s.Data.Timestamp)                                            // 2
	f("\n")                                                                                       // 3
	f("\tBudget\n")                                                                               // 4
	f("\tACME Token limit\t%d\n", s.Data.TokenLimit)                                              // 5
	f("\tACME Tokens Issued\t%d\n", s.Data.TokensIssued)                                          // 6
	f("\tToken Issuance Rate\t%d%%\n", s.Data.TokenIssuanceRate)                                  // 7
	f("\tAnnual Budget\t=round((C5-C6)*C7,4)\n")                                                  // 8
	f("\tWeekly Budget\t=round(C8/365.25*7,4)\n")                                                 // 9
	f("\tAPR\t=c8/c21\n")                                                                         // 10
	f("\t\n")                                                                                     // 11
	f("\tWeight Pure Staking Account\t%d%%\n", s.Data.WtPS)                                       // 12
	f("\tWeight Protocol Validator\t%d%%\n", s.Data.WtPV)                                         // 13
	f("\tWeight Protocol Follower\t%d%%\n", s.Data.WtPF)                                          // 14
	f("\tWeight Staking Validator\t%d%%\n", s.Data.WtSV)                                          // 15
	f("\n")                                                                                       // 16
	f("\tDelegated Account Return\t%d%%\n", s.Data.DelegatedAccountReturn)                        // 17
	f("\tStaking Share of Delegated Tokens\t%d%%\n", s.Data.DelegateeShare)                       // 18
	f("\n")                                                                                       // 19
	f("\t\tPure Tokens\tWeighted Tokens\tReward\n")                                               // 20
	f("\tTotal Staked Tokens\t%d\t%d\t=C9\n", s.Data.TotalTokens, s.Data.TotalWeighted)           // 21
	fm := "\tTotal %s Accounts\t%d\t%d\t=round(C$9/D$21*%s,4)\n"                                  //
	m := func(tokens int64, wt float64) int64 { return int64(float64(tokens) * wt / 100) }        //
	f(fm, "Pure Staking", s.Data.TotalPSD, m(s.Data.TotalPSD, float64(s.Data.WtPS)), "D22")       // 22
	f(fm, "Protocol Validator", s.Data.TotalPVD, m(s.Data.TotalPVD, float64(s.Data.WtPV)), "D23") // 23
	f(fm, "Protocol Follower", s.Data.TotalPFD, m(s.Data.TotalPFD, float64(s.Data.WtPF)), "D24")  // 24
	f(fm, "Staking Validator", s.Data.TotalSVD, m(s.Data.TotalSVD, float64(s.Data.WtSV)), "D25")  // 25
	f("\n")                                                                                       // 26
	f("\tTotal Weighted Staked Tokens\t=D21\n")                                                   // 27
	f("\tReward per Pure Staking token\t=round(E22/C22,7)\n")                                     // 28
	f("\tReward per Protocol Validator Token\t=round(E23/C23,7)\n")                               // 29
	f("\tReward per Protocol Follower Token\t=round(E24/C24,7)\n")                                // 30
	f("\tReward per Staking Validator Token\t=round(E25/C25,7)\n")                                // 31
	f("\t\n")                                                                                     // 32
	f("\tAPR Pure Staking\t=round(E22*(365.25/7)/c22,4)\n")                                       // 33
	f("\tAPR Protocol Validator Staking\t=round(E23*(365.25/7)/c23,4)\n")                         // 34
	f("\tAPR Protocol Follower Staking\t=round(E24*(365.25/7)/c24,4)\n")                          // 35
	f("\tAPR Staking Validator Staking\t=round(E25*(365.25/7)/c25,4)\n")                          // 36
	f("\n")                                                                                       // 37
	f("\n")                                                                                       // 38
	lines, end := s.PrintAccounts(PureStaker, "Pure Staking Accounts", s.Stakers.Pure, "c12", "c28", 39)
	f(lines)
	lines, end = s.PrintAccounts(ProtocolValidator, "Protocol Validator Accounts", s.Stakers.PValidator, "c13", "c29", end)
	f(lines)
	lines, end = s.PrintAccounts(ProtocolFollower, "Protocol Follower Accounts", s.Stakers.PFollower, "c14", "c30", end)
	f(lines)
	lines, _ = s.PrintAccounts(StakingValidator, "Staking Validator Accounts", s.Stakers.SValidator, "c15", "c31", end)
	f(lines)

	fmt.Print(report.String())
	reportFile := path.Join(ReportDirectory, fmt.Sprintf("Report-%d.csv", s.Data.BlockHeight))
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

// Writes out all the lines for a Staking Category
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
	f("\tURL\tAccount Tokens\tTotal Tokens\tWeighted Tokens\tTotal Reward\tAccount Reward\n")
	for _, a := range accounts {
		if a.Type == Type && a.MajorBlock < s.CBlk.MajorHeight {
			dl := len(a.Delegates) // Calculate the number of lines for delegates
			// URL | BALANCE | =Balance+sum | (Balance+sum)*(WEIGHT) | Total REWARD | Account Reward
			if len(a.Delegates) > 0 {
				fm := "\t%s" +
					"\t=round(%d,4)" +
					"\t=sum(C%d:C%d)" +
					"\t=round(D%d*%s,4)" +
					"\t=round(E%d*%s,4)" +
					"\t=round(C%d*%s,4)+sum(C%d:C%d)*%s*%s*C18\n"
				f(fm,
					a.URL.String(),
					a.Balance,
					end, end+dl,
					end, weight,
					end, reward,
					end, reward, end+1, end+dl, reward, weight)
				fd := "Delegate\t%s\t=round(%d,4)\t\t\t\t=int(C%d*%s*%s*C17)\n"
				for _, d := range a.Delegates {
					f(fd, d.URL.String(), d.Balance, end, weight, reward)
				}

			} else {
				fm := "\t%s\t=round(%d,4)\t=sum(C%d:C%d)\t=round(D%d*%s,4)\t=round(E%d*%s,4)\t=round(C%d*%s,4)\n"
				f(fm,
					a.URL.String(),
					a.Balance,
					end, end+dl,
					end, weight,
					end, reward,
					end, reward)
			}
		}
	}
	return report.String(), end
}

// AddApproved
// Look in the block at the Approved Account, and add any new entries
func (s *StakingApp) AddApproved(b *Block) {
	approved := b.GetAccount(Approved)
	if approved == nil {
		return
	}
	for _, v := range approved.Entries {
		account := v.(*Account)
		switch account.Type {
		case PureStaker:
			s.Stakers.Pure = append(s.Stakers.Pure, account)
		case ProtocolValidator:
			s.Stakers.PValidator = append(s.Stakers.PValidator, account)
		case ProtocolFollower:
			s.Stakers.PFollower = append(s.Stakers.PFollower, account)
		case StakingValidator:
			s.Stakers.SValidator = append(s.Stakers.SValidator, account)
		default:
			panic(fmt.Sprintf("Unknown account type: %v", account.Type))
		}
	}
	sort.Slice(s.Stakers.Pure, func(i, j int) bool { return s.Stakers.Pure[i].URL.String() < s.Stakers.Pure[j].URL.String() })
	sort.Slice(s.Stakers.Pure, func(i, j int) bool { return s.Stakers.Pure[i].URL.String() < s.Stakers.Pure[j].URL.String() })
	sort.Slice(s.Stakers.Pure, func(i, j int) bool { return s.Stakers.Pure[i].URL.String() < s.Stakers.Pure[j].URL.String() })
	sort.Slice(s.Stakers.Pure, func(i, j int) bool { return s.Stakers.Pure[i].URL.String() < s.Stakers.Pure[j].URL.String() })
}
