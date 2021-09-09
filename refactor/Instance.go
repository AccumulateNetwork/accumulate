package refactor

// Instance
// Define the instance of the Accumulate servers being run as defined by the
// configuration file.  We allow multiple instances of nodes to be run
// together to limit hardware required to run the protocol when the transaction
// rate doesn't require too much hardware.
type Instance struct {
	DVC      *DCNode
	BVCs     []*BVCNode
	ApiInput chan []byte
}

func (ins Instance) SemdMessage(t *GenTransaction) {
	if ins.DVC != nil {
		ins.DVC.SendMessage(t)
	} else if len(ins.BVCs) > 0 {
		ins.BVCs[0].SendMessage(t)
	}
}

func (ins Instance) Process() {
	for {
		t := <-ins.ApiInput
		gt := new(GenTransaction)
		gt.UnMarshal(t)
		ins.SemdMessage(gt)
	}
}
