package sim

import (
	"fmt"
	"testing"
)

func TestGenUrl(t *testing.T) {
	for i := 0; i < 50; i++ {
		adi, url := GenUrls("StakingAccount")
		fmt.Printf("%35s | %-55s\n", adi, url)
	}
}


func TestGenerateInitializationScript(t *testing.T) {
	fmt.Println ("lta=acc://c83b1ed6b8b6795d3c224dab50a544e2306d743866835260/ACME")
	fmt.Printf("for i in {0..5}\ndo\n   echo asdfasdf | accumulate faucet $lta\ndone\n")	// accumulate credits [origin token account] [key page or lite identity url] [number of credits wanted] [max acme to spend] [percent slippage (optional)] [flags][BS2]
	fmt.Print("echo asdfasdf | accumulate credits $lta $lta 500000\n\n")
	for i := 0; i < 1; i++ {
		adi, url := GenUrls("StakingAccount")
		fmt.Printf("echo asdfasdf | accumulate adi create $lta %s masterkey\n",adi)
		// accumulate account create token [actor adi] [signing key name] [key index (optional)] [key height (optional)] [new token account url] [tokenUrl] [keyBook (optional)] [flags]
		// ./accumulate account create token acc://DefiDevs.acme Key1 acc://DefiDevs.acme/Tokens acc://ACME acc://DefiDevs.acme/Keybook

		fmt.Printf("echo asdfasdf | accumulate account create token $lta masterkey %s\n",url)

		fmt.Println("")
	}
}

