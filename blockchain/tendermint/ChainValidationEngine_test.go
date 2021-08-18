package tendermint

import (
	"fmt"
	"testing"
)

func TestChainValidationEngine(t *testing.T) {
	done := make(chan bool, 1)
	go ChainValidationEngine(done)

	<-done

	fmt.Printf("All done...")
}
