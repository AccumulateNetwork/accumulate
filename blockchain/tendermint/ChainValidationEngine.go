package tendermint

import (
	"fmt"
	"time"
)

func ChainValidationEngine(complete chan bool) {

	fmt.Print("working...")
	time.Sleep(time.Second)
	fmt.Println("done")

	///	msg <-
	complete <- true
}
