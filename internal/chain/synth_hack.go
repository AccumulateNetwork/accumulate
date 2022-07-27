package chain

import (
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var synthForAnchor = struct {
	sync.Mutex
	values map[string]map[[32]byte][]*protocol.Envelope
}{
	values: map[string]map[[32]byte][]*protocol.Envelope{},
}

func AddSynthForAnchor(pid string, anchor [32]byte, txn *protocol.Transaction, sigs ...protocol.Signature) {
	synthForAnchor.Lock()
	defer synthForAnchor.Unlock()
	v := synthForAnchor.values[pid]
	if v == nil {
		v = map[[32]byte][]*protocol.Envelope{}
		synthForAnchor.values[pid] = v
	}
	v[anchor] = append(v[anchor], &protocol.Envelope{Transaction: []*protocol.Transaction{txn}, Signatures: sigs})
}

func GetSynthForAnchor(pid string, anchor [32]byte) []*protocol.Envelope {
	synthForAnchor.Lock()
	defer synthForAnchor.Unlock()
	return synthForAnchor.values[pid][anchor]
}
