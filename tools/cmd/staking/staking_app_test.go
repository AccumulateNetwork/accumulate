package main

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestStakingApp_PrintAccounts(t *testing.T) {
	s := new(StakingApp)
	s.CBlk=&Block{MajorHeight: 3}
	accounts := []*Account{}
	for i:=0;i<4;i++{
		account := new(Account)
		account.MajorBlock=0
		account.URL = GenAccount("tokens")
		account.Type = PureStaker
		account.Balance=rand.Int63()%10000+25000
		accounts = append(accounts,account)
	}
	gotLines, gotEnd := s.PrintAccounts(PureStaker, "Pure Staking Accounts", accounts,"c12", "c28", 39)
	fmt.Println(gotEnd)
	fmt.Println(gotLines)
}
