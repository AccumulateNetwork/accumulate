package sim

import (
	"bytes"
	"fmt"
	"os"
	"os/user"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type staker struct {
	adi *url.URL
	url *url.URL
}

var stakers []*staker

func TestGenUrl(t *testing.T) {
	for i := 0; i < 50; i++ {
		adi, url := GenUrls("StakingAccount")
		fmt.Printf("%35s | %-55s\n", adi, url)
	}
}

func TestGenerateInitializationScript(t *testing.T) {
	buff := bytes.Buffer{}
	pf := func(format string, a ...any) {
		format += "\n"
		buff.WriteString(fmt.Sprintf(format, a...))
	}
	p := func(a ...any) {
		a = append(a, "\n")
		buff.WriteString(fmt.Sprint(a...))
	}
	cr := func() {
		buff.WriteString("echo\n")
	}

	GenRH.SetSeed([]byte{3,2,3,5})

	for i := 0; i < 2; i++ {
		adi, url := GenUrls("StakingAccount")
		stakers = append(stakers, &staker{adi: adi, url: url})
	}

	p("export ACC_API=http://127.0.1.1:26660/v2 ")
	p("lta=acc://c83b1ed6b8b6795d3c224dab50a544e2306d743866835260/ACME")
	pf("for i in {0..200}\ndo\n   echo asdfasdf | accumulate faucet $lta\ndone") // accumulate credits [origin token account] [key page or lite identity url] [number of credits wanted] [max acme to spend] [percent slippage (optional)] [flags][BS2]
	p("sleep 5s")
	p("echo asdfasdf | accumulate credits $lta $lta 500000")
	p("sleep 15s")
	cr()
	p("echo set up staking rewards")
	p("echo asdfasdf | accumulate adi create $lta acc://staking.acme masterkey")
	p("sleep 10s")
	cr()
	p("echo get credits")
	p("echo asdfasdf | accumulate credits $lta acc://staking.acme/book/1 500000")
	p("sleep 10s")
	cr()
	p("echo set up data accounts")
	p("echo asdfasdf | accumulate account create data acc://staking.acme masterkey acc://staking.acme/Approvedx")
	p("echo asdfasdf | accumulate account create data acc://staking.acme masterkey acc://staking.acme/Registeredx")
	p("echo asdfasdf | accumulate account create data acc://staking.acme masterkey acc://staking.acme/Disputesx")
	cr()
	p("set up staking ADIs")
	for _, v := range stakers {
		pf("echo Create ADI %s", v.adi)
		pf("echo asdfasdf | accumulate adi create $lta %s masterkey", v.adi)
	}
	cr()
	p("sleep 15s")
	for _, v := range stakers {
		pf("echo Add credits to ADI %s/book/1", v.adi)
		pf("echo asdfasdf | accumulate credits $lta %s/book/1 10000", v.adi)
	}
	cr()
	p("sleep 15s") // Have to wait for credits to settle
	for _, v := range stakers {
		pf("echo Create Token account %s", v.url)
		pf("echo asdfasdf | accumulate account create token %s masterkey %s acc://acme", v.adi, v.url)
	}
	cr()
	p("sleep 15s") // Have to wait for credits to settle
	for _, v := range stakers {
		pf("echo Move tokens from $lta to %s amount %d", v.url, 40)
		pf("echo asdfasdf | accumulate tx create $lta %s %d", v.url, 40)
	}

	

	fmt.Print(buff.String())
	u, _ := user.Current()
	scriptName := path.Join(u.HomeDir, "tmp", "staking", "initScaling.sh")
	file, err := os.Create(scriptName)
	require.NoError(t, err)
	file.WriteString(buff.String())

	// Add all the Staking accounts to the approved list


}
