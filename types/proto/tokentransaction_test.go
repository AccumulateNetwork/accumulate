package proto

import "testing"

func TestTokenSend(t *testing.T) {
	nts1 := NewTokenSend("RedWagon", Output{100 * 100000000, "BlueWagon"})
	nts2 := NewTokenSend("RedWagon",
		Output{100 * 100000000, "YellowWagon"},
		Output{100 * 100000000, "BrownWagon"},
		Output{100 * 100000000, "GreenWagon"})
	nts1data := nts1.Marshal()
	nts2data := nts2.Marshal()
	nts1b := new(TokenSend)
	nts2b := new(TokenSend)
	_ = nts1b.Unmarshal(nts1data)
	_ = nts2b.Unmarshal(nts2data)

	if !nts1.Equal(nts1b) || nts1.Equal(nts2b) {
		t.Error("check expected relationship between nts1 and nts2")
	}
}
