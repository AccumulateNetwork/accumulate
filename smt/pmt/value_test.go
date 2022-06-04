package pmt

import (
	"fmt"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/smt/common"
)

func TestValue_Equal(t *testing.T) {
	var rh common.RandHash
	bpt := NewBPTManager(nil).Bpt        //     Build a BPT
	c := rh.NextA()
	v := bpt.NewValue(nil,c,c,rh.NextA())
	v2 := new(Value)
	v2.Copy(v)

	if !v.Equal(v2) {
		t.Error("value should be the same")
	}
}

func prtAll(n Entry){
	node,isNode := n.(*BptNode)
	value,isValue := n.(*Value)
	switch{
	case isNode:
		prtAll(node.Left)
		prtAll(node.Right)
	case isValue:
		fmt.Printf(" adi %x account %x key %x hash %x\n",
		value.ADI[:3],value.Account[:3],value.Key[:3],value.Hash[:3])
	}
}

func TestValue_BPT(t *testing.T){
	bpt := NewBPTManager(nil).Bpt
	for i:=255;i>=0;i--{
		c1 := [32]byte{byte(i)}
		c2 := [32]byte{byte(i)}
		c3 := [32]byte{byte(i)}
		bpt.manager.InsertKV(c1,c2,c3)
	}
	bpt.Update()
	prtAll(bpt.GetRoot())
}