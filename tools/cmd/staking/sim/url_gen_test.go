package sim

import (
	"fmt"
	"testing"
)

func TestGenUrl(t *testing.T) {
	for i := 0; i < 100; i++ {
		adi, url := GenUrls("StakingAccount")
		fmt.Printf("%35s | %-55s\n", adi, url)
	}
}
