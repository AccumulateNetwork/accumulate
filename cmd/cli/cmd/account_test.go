package cmd

import (
	"fmt"
	"testing"
)

//unitTest1_1 Generate 100 lite accounts using cli that can be used for other tests
func unitTest1_1(t *testing.T) {
	for i := 0; i < 100; i++ {
		fmt.Sprintf("account generate")
	}

}

//TestGenerateAccount
func TestGenerateAccount(t *testing.T) {

}
