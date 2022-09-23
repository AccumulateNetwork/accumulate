package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/tools/cmd/staking/app"
)

func main() {
	params := new(app.Parameters)
	err := json.NewDecoder(os.Stdin).Decode(params)
	if err != nil {
		log.Fatal(err)
	}

	b, err := params.MarshalBinary()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%x\n", b)
}
