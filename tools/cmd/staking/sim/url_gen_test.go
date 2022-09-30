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
	fmt.Println("lta=acc://c83b1ed6b8b6795d3c224dab50a544e2306d743866835260/ACME")
	fmt.Printf("for i in {0..1}\ndo\n   echo asdfasdf | accumulate faucet $lta\ndone\n") // accumulate credits [origin token account] [key page or lite identity url] [number of credits wanted] [max acme to spend] [percent slippage (optional)] [flags][BS2]
	fmt.Print("echo asdfasdf | accumulate credits $lta $lta 500000\n\n")
	for i := 0; i < 1; i++ {
		adi, url := GenUrls("StakingAccount")
		fmt.Printf("echo asdfasdf | accumulate adi create $lta %s masterkey\n", adi)
		// accumulate account create token [actor adi] [signing key name] [key index (optional)] [key height (optional)] [new token account url] [tokenUrl] [keyBook (optional)] [flags]
		// ./accumulate account create token acc://DefiDevs.acme Key1 acc://DefiDevs.acme/Tokens acc://ACME acc://DefiDevs.acme/Keybook
		fmt.Printf("echo asdfasdf | accumulate account create token %s masterkey %s acc://acme\n", adi, url)
		// accumulate tx create [origin url] [signing key name] [key index (optional)] [key height (optional)] [to] [amount] Create new token tx
		// ./accumulate tx create acc://27259b5544176ede283ba745b5a6d1775a281ad2f28abc3d/ACME acc://DefiDevs.acme/Tokens 100
		fmt.Printf("echo asdfasdf | accumulate tx create $lta %s %d\n", url, 40)
		fmt.Println("")
	}
}
