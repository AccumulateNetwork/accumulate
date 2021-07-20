package tendermint

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"github.com/AccumulateNetwork/SMT/managed"
	"github.com/AccumulateNetwork/SMT/pmt"
	smtdb "github.com/AccumulateNetwork/SMT/storage/database"
	"github.com/AccumulateNetwork/accumulated/api/proto"
	"github.com/AccumulateNetwork/accumulated/blockchain/validator"
	"github.com/AccumulateNetwork/accumulated/blockchain/validator/types"
	"time"
)


const MaxMessageSize = 10270
//type Message proto.Submission

//probably need the base type


type ChainValidationCandidate struct {
	StateEntry validator.StateEntry
	Data []byte
}

type CVE struct {
    channelid int
	mmdb *smtdb.Manager
	mms *map[managed.Hash]*MerkleManagerState
	bpt *pmt.Manager


}

func (app *CVE) getCurrentState(chainid []byte) (*types.StateObject, error) {
	var ret *types.StateObject
	var key managed.Hash
	key.Extract(chainid)
	if mms := app.mms[key]; mms != nil {
		ret = &mms.currentstateobject
	} else {
		//pull current state from the database.
		data := app.mmdb.Get("StateEntries", "", chainid)
		if data != nil {
			ret = &types.StateObject{}
			err := ret.Unmarshal(data)
			if err != nil {
				return nil, fmt.Errorf("No Current State is Defined")
			}

		}
	}
	return ret,nil
}


func (app *CVE) addStateEntry(chainid []byte, entry []byte) error {
	var mms *MerkleManagerState

	hash := sha256.Sum256(entry)
	key := managed.Hash{}
	key.Extract(chainid)
	if mms = app.mms[key]; mms == nil {
		mms = new(MerkleManagerState)
		mms.merklemgr = managed.NewMerkleManager(&app.mmdb,chainid,8)
		app.mms[key] = mms
	}
	data := app.mmdb.Get("StateEntries", "", chainid)
	if data != nil {
		currso := types.StateObject{}
		mms.currentstateobject.PrevStateHash = currso.PrevStateHash
	}
	mms.merklemgr.AddHash(hash)
	mdroot := mms.merklemgr.MainChain.MS.GetMDRoot()

	//The Entry feeds the Entry Hash, and the Entry Hash feeds the State Hash
	//The MD Root is the current state
	mms.currentstateobject.StateHash = mdroot.Bytes()
	//The Entry hash is the hash of the state object being stored
	mms.currentstateobject.EntryHash = hash[:]
	//The Entry is the State object derived from the transaction
	mms.currentstateobject.Entry = entry

	//list of the state objects from the beginning of the block to the end, so don't know if this needs to be kept
	mms.stateobjects = append(mms.stateobjects, mms.currentstateobject )
	return nil
}


func (app *CVE) validationChainWorker(q chan ChainValidationCandidate, e chan error) {

	for ;; {
		se := <- q

		//e <- fmt.Errorf("Error unmarshaling message for error identity (%X) chain id (%X)", se.ChainState.Entry, sub.GetChainid())


		//sub.Identitychain
		//sub.Chainid

		//need to resolve public key.

		//resolve public key from identity

		var is types.IdentityState{}
		is.UnmarshalBinary(se.IdentityState.Entry)

		chain :=
		ed25519.Verify(is.Publickey,se..Data,sub.Signature)

	}
}

func (app *CVE) validateChain() {

}

//Because chains are actec upon in isolation during an update, we can take advantage of this by splitting the validation
//by chain amongst CPU's. Only data operating on similar chains but be run in the same thread to ensure ordering
//of chain operations is preserved.
func ChainValidationEngine(complete chan bool) {
	//
	//type q chan uint8
	//
	//channels := make([]q,10) //10 messages queued per channel
	//
	//for i, _ := range channels {
	//	channels[i] = make(chan uint8, MaxMessageSize)
	//}
	//
	//go

	qchan := make(chan ChainValidationCandidate, 100)
	echan := make(chan error)

	var cve CVE

	go cve.validationChainWorker(qchan,echan)



    fmt.Print("working...")
    time.Sleep(time.Second)
    fmt.Println("done")

///	msg <-
    complete <- true
}
