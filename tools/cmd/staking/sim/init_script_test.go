package sim

import (
	"bytes"
	"fmt"
	"os"
	"os/user"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	skip "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/tools/cmd/staking/app"
)

// TestGenerateInitializationScript(t *testing.T)
// Creates script files and does some limited testing of Accumulate
// Could possibly move to a utility, but its kinda nice to run as a unit test
func TestGenerateInitializationScript(t *testing.T) {
	skip.SkipCI(t, "support for manual testing")

	GenRH.SetSeed([]byte{6})
	seconds := 15

	buff := bytes.Buffer{}
	pf := func(format string, a ...any) { //          Support for formatted printing + \n
		format += "\n"
		buff.WriteString(fmt.Sprintf(format, a...))
	}
	p := func(a ...any) { //                          Support for simple printing + \n
		a = append(a, "\n")
		buff.WriteString(fmt.Sprint(a...))
	}
	cr := func() { //                                 Put out a blank line \n
		buff.WriteString("echo\n")
	}
	w := func(a ...any) { //                          Simple printing with no \n
		buff.WriteString(fmt.Sprint(a...))
	}
	_ = w
	s := func() {
		pf("sleep %d", seconds)
	}

	for i := 0; i < 10; i++ {
		adi, url := GenUrls("StakingAccount")
		stakers = append(stakers, &staker{adi: adi, url: url})
	}

	p("export ACC_API=http://127.0.1.1:26660/v2 ")
	p("lta=acc://c83b1ed6b8b6795d3c224dab50a544e2306d743866835260/ACME")
	pf("for i in {0..200}\ndo\n   echo asdfasdf | accumulate faucet $lta\ndone") // accumulate credits [origin token account] [key page or lite identity url] [number of credits wanted] [max acme to spend] [percent slippage (optional)] [flags][BS2]
	s()
	p("echo asdfasdf | accumulate credits $lta $lta 500000")
	s()
	cr()
	p("echo set up staking rewards")
	p("echo asdfasdf | accumulate adi create $lta acc://staking.acme masterkey")
	s()
	cr()
	p("echo get credits")
	p("echo asdfasdf | accumulate credits $lta acc://staking.acme/book/1 500000")
	s()
	cr()
	p("echo set up data accounts")
	p("echo asdfasdf | accumulate account create data acc://staking.acme masterkey acc://staking.acme/Approved")
	p("echo asdfasdf | accumulate account create data acc://staking.acme masterkey acc://staking.acme/Registered")
	p("echo asdfasdf | accumulate account create data acc://staking.acme masterkey acc://staking.acme/Disputes")
	cr()
	p("set up staking ADIs")
	for _, v := range stakers {
		pf("echo Create ADI %s", v.adi)
		pf("echo asdfasdf | accumulate adi create $lta %s masterkey", v.adi)
	}
	cr()
	s()
	for _, v := range stakers {
		pf("echo Add credits to ADI %s/book/1", v.adi)
		pf("echo asdfasdf | accumulate credits $lta %s/book/1 10000", v.adi)
	}
	cr()
	p("sleep 15s") // Have to wait for credits to settle
	for _, v := range stakers {
		pf("echo Create Token account %s", v.url)
		pf("echo asdfasdf | accumulate account create token %s masterkey %s acc://acme", v.adi, v.url)
		pf("echo Create Token account %s", v.adi.String()+"/rewards")
		pf("echo asdfasdf | accumulate account create token %s masterkey %s acc://acme", v.adi, v.adi.String()+"/rewards")
	}
	cr()
	for _, v := range stakers {
		pf("echo Move tokens from $lta to %s amount %d", v.url, 40)
		pf("echo asdfasdf | accumulate tx create $lta %s %d", v.url, 40)
	}

	Accounts := AllocateStakers(t)

	cr()
	p("echo add staking accounts to Approved")
	for _, a := range Accounts {
		deposit := a.DepositURL.String()
		if GenRH.Next()[0] > 128 {
			deposit = a.URL.String()
		}
		pf("echo asdfasdf | accumulate data write acc://staking.acme/Approved masterkey \"%s\" \"%s\" \"%s\"", a.URL, deposit, a.Type)
	}

	cr()
	p("#---------------- Grow staking accounts --------------------")
	pf("for i in {0..200}\ndo\n   echo asdfasdf | accumulate faucet $lta\ndone") // accumulate credits [origin token account] [key page or lite identity url] [number of credits wanted] [max acme to spend] [percent slippage (optional)] [flags][BS2]
	p("for i in {0..100000}")
	p("do")
	for _, v := range stakers {
		pf("echo asdfasdf | accumulate faucet $lta")
		pf("echo asdfasdf | accumulate tx create $lta %s %d", v.url, 40)
		p("sleep .3s")
	}
	p("done")

	fmt.Print(buff.String())
	u, _ := user.Current()
	scriptName := path.Join(u.HomeDir, "tmp", "staking", "initScaling.sh")
	file, err := os.Create(scriptName)
	require.NoError(t, err)
	file.WriteString(buff.String())

	// Add all the Staking accounts to the approved list

}

func AllocateStakers(t *testing.T) (accounts []*app.Account) {
	currentStaker := new(app.Account)
	currentStaker.Type = app.PureStaker
	var err error
	for _, v := range stakers {
		staker := new(app.Account)
		accounts = append(accounts, staker)
		staker.URL = v.url
		staker.DepositURL, err = url.Parse(v.adi.String() + "/rewards")
		require.NoError(t, err)

		for {
			switch GenRH.GetRandInt64() % 5 {
			case 0:
				staker.Type = app.PureStaker
				currentStaker = staker
			case 1:
				staker.Type = app.ProtocolValidator
				currentStaker = staker
			case 2:
				staker.Type = app.ProtocolFollower
				currentStaker = staker
			case 3:
				staker.Type = app.StakingValidator
				currentStaker = staker
			case 4:
				if currentStaker == nil { // Got a delegate, but need something else first
					continue //              So try again.
				}
				staker.Type = app.Delegate
				currentStaker.Delegates = append(currentStaker.Delegates, staker)
				staker.Delegatee = currentStaker
			default:
				panic("should never happen")
			}
			break // We are in this loop until we get something other than a Delegate.
		}
	}
	return accounts
}
