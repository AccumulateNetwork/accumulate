package abci

import (
	"fmt"

	"github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/merkleDag"
	valacc "github.com/AccumulateNetwork/ValidatorAccumulator/ValAcc/types"
	"github.com/AccumulateNetwork/accumulated/types/proto"
	protobuf "github.com/golang/protobuf/proto"
	"github.com/tendermint/tendermint/abci/example/code"
	abci "github.com/tendermint/tendermint/abci/types"
	"golang.org/x/crypto/ed25519"
)

// Directory is an ABCI application that implements an Accumulate directory
// chain.
type Directory struct {
	abci.BaseApplication
	md        merkleDag.MD
	appMDRoot valacc.Hash
}

var _ abci.Application = (*Directory)(nil)

func (Directory) Info(abci.RequestInfo) abci.ResponseInfo {
	return abci.ResponseInfo{}
}

func (app *Directory) resolveDDIIatHeight(ddii []byte, bvcheight int64) (ed25519.PublicKey, error) {
	//just give me a key...

	// fmt.Printf("%s", string(ddii[:]))
	//TODO: need to find out what the public key for ddii was at height bvcheight
	//only temporary... create a valid key
	//The DBVC will subscribe to the DDII - BVC nodes and will cache the latest valid public key for the BVC DDII
	pub, _, err := ed25519.GenerateKey(nil)
	return pub, err
}

func (app *Directory) verifyBVCMasterChain(ddii []byte) error {

	//make sure we're dealing with a valid registered BVC master chain, not just any ol' chain.
	//the BVC chains will be managed and registered by the DBVC.
	return nil
}

//InitChain will get called at the initialization of the dbvc
func (app *Directory) InitChain(abci.RequestInitChain) abci.ResponseInitChain {
	// fmt.Printf("Initalizing Accumulator Router\n")

	//TODO: do a load state here to continue on with where we were.
	//loadState(...)
	//reset height to last good height and restore app.AppMDRoot

	//TODO query something to resolve all BVC Master Chains to map ddii's to pub keys
	//wood be good to cache the ddii's or at least observe the DDII chain to quickly resolve those.

	//TODO: rebuild MD from store, for now just initialize it.
	app.md.AddToChain(app.appMDRoot)

	return abci.ResponseInitChain{}
}

//BeginBlock Here we create a batch, which will store block's transactions.
// ------ BeginBlock() -> DeliverTx()... -> EndBlock() -> Commit()
// When Tendermint Core has decided on the block, it's transferred to the application in 3 parts:
// BeginBlock, one DeliverTx per transaction and EndBlock in the end.
func (app *Directory) BeginBlock(abci.RequestBeginBlock) abci.ResponseBeginBlock {
	//probably don't need to do this here...
	//app.AppMDRoot.Extract(req.Hash)

	return abci.ResponseBeginBlock{}
}

// CheckTx BVC Block is finished and MDRoot data is delivered to DBVC. Check if it is valid.
func (app *Directory) CheckTx(req abci.RequestCheckTx) abci.ResponseCheckTx {
	//the ABCI request here is a Tx that consists data delivered from the BVC protocol buffer
	//data here can only come from an authorized VBC validator, otherwise they will be rejected
	//Step 1: check which BVC is sending the request and see if it is a valid Master Chain.
	header := proto.DBVCInstructionHeader{}

	err := protobuf.Unmarshal(req.GetTx(), &header)
	if err != nil {
		return abci.ResponseCheckTx{Code: code.CodeTypeEncodingError, GasWanted: 0}
	}

	err = app.verifyBVCMasterChain(header.GetBvcMasterChainDDII())
	if err != nil { //add validation here.
		//quick filter to see if the request if from a valid master chain
		return abci.ResponseCheckTx{Code: code.CodeTypeUnauthorized, GasWanted: 0}
	}

	switch header.GetInstruction() {
	case proto.DBVCInstructionHeader_EntryReveal:
		//Step 2: resolve DDII of BVC against VBC validator
		bvcreq := proto.BVCEntry{}

		err = protobuf.Unmarshal(req.GetTx(), &bvcreq)

		if err != nil {
			return abci.ResponseCheckTx{Code: code.CodeTypeEncodingError, GasWanted: 0,
				Log: "Unable to decode BVC Protobuf Transaction"}
		}

		bve := directoryEntry{}
		_, err = bve.UnmarshalBinary(bvcreq.GetEntry())
		if err != nil {
			return abci.ResponseCheckTx{Code: code.CodeTypeUnauthorized, GasWanted: 0,
				Log: fmt.Sprintf("Unable to resolve DDII at Height %d", bve.BVCHeight)}
		}
		//resolve the validator's bve to obtain public key for given height
		pub, err := app.resolveDDIIatHeight(bve.DDII, bve.BVCHeight)
		if err != nil {
			return abci.ResponseCheckTx{Code: code.CodeTypeUnauthorized, GasWanted: 0,
				Log: fmt.Sprintf("Unable to resolve DDII at Height %d", bve.BVCHeight)}
		}

		//Step 3: validate signature of signed accumulated merkle dag root
		if !ed25519.Verify(pub, bvcreq.GetEntry(), bvcreq.GetSignature()) {
			println("Invalid Signature")
			return abci.ResponseCheckTx{Code: code.CodeTypeUnauthorized, GasWanted: 0,
				Log: "Invalid Signature"}
		}
	default:
		return abci.ResponseCheckTx{Code: code.CodeTypeEncodingError, GasWanted: 0, Log: "Bad Instruction Header"}

	}
	//Step 4: if signature is valid send dispatch to accumulator directory block
	return abci.ResponseCheckTx{Code: code.CodeTypeOK, GasWanted: 1}
}

// Invalid transactions, we again return the non-zero code.
// Otherwise, we add it to the current batch.
func (app *Directory) DeliverTx(req abci.RequestDeliverTx) (response abci.ResponseDeliverTx) {

	//if we get this far, than it has passed check tx,
	bvcreq := proto.BVCEntry{}
	err := protobuf.Unmarshal(req.GetTx(), &bvcreq)
	if err != nil {
		return abci.ResponseDeliverTx{Code: 2, GasWanted: 0}
	}

	bve := directoryEntry{}
	entry_slices, _ := bve.UnmarshalBinary(bvcreq.GetEntry())

	bvcheight := bve.BVCHeight

	//resolve the validator's bve to obtain public key for given height
	bvcpubkey, err := app.resolveDDIIatHeight(bve.DDII, bvcheight)
	if err != nil {
		return abci.ResponseDeliverTx{Code: 2, GasWanted: 0}
	}
	//everyone verify...

	if ed25519.Verify(bvcpubkey, bvcreq.GetEntry(), bvcreq.GetSignature()) {
		println("Invalid Signature")
		return abci.ResponseDeliverTx{Code: 3, GasWanted: 0}
	}

	mdr := valacc.Hash{}
	copy(mdr.Bytes(), bve.MDRoot.Bytes())

	app.md.AddToChain(mdr)

	//index the events to let BVC know MDRoot has been secured so that consensus can be achieved by BVCs
	response.Events = []abci.Event{
		{
			Type: "bvc",
			Attributes: []abci.EventAttribute{
				//want to be able to search by BVC chain.
				{Key: []byte("chain"), Value: bvcreq.GetHeader().GetBvcMasterChainDDII(), Index: true},
				//want to be able to search by height, but probably should be AND'ed with the chain
				{Key: []byte("height"), Value: entry_slices[BVCHeight_type], Index: true},
				//want to be able to search by ddii (optional AND'ed with chain or height)
				{Key: []byte("ddii"), Value: entry_slices[DDII_type], Index: true},
				//don't care about searching by bvc timestamp or valacc hash
				{Key: []byte("timestamp"), Value: entry_slices[Timestamp_type], Index: false},
				{Key: []byte("mdroot"), Value: entry_slices[MDRoot_type], Index: false},
			},
		},
	}
	response.Code = code.CodeTypeOK
	return response
}

func (app *Directory) EndBlock(abci.RequestEndBlock) abci.ResponseEndBlock {
	//todo: validator adjustments here...
	//todo: do consensus adjustments here...
	//Signals the end of a block.
	//	Called after all transactions, prior to each Commit.
	//	Validator updates returned by block H impact blocks H+1, H+2, and H+3, but only effects changes on the validator set of H+2:
	//		H+1: NextValidatorsHash
	//		H+2: ValidatorsHash (and thus the validator set)
	//		H+3: LastCommitInfo (ie. the last validator set)
	//	Consensus params returned for block H apply for block H+1
	return abci.ResponseEndBlock{}
}

//Commit instructs the application to persist the new state.
func (app *Directory) Commit() abci.ResponseCommit {
	//TODO: Determine if folding in prev block hash necessary
	app.appMDRoot = *app.md.GetMDRoot().Combine(app.appMDRoot)

	//TODO: saveState(app.appmdroot, currentheight);
	return abci.ResponseCommit{Data: app.appMDRoot.Bytes()}
}

//------------------------

// when the client wants to know whenever a particular key/value exist, it will call Tendermint Core RPC /abci_query endpoint
func (app *Directory) Query(reqQuery abci.RequestQuery) (resQuery abci.ResponseQuery) {
	resQuery.Key = reqQuery.Data
	/*
		err := app.db.View(func(txn *badger.Txn) error {
			item, err := txn.Get(reqQuery.Data)
			if err != nil && err != badger.ErrKeyNotFound {
				return err
			}
			if err == badger.ErrKeyNotFound {
				resQuery.Log = "does not exist"
			} else {
				return item.Value(func(val []byte) error {
					resQuery.Log = "exists"
					resQuery.Value = val
					return nil
				})
			}
			return nil
		})
		if err != nil {
			panic(err)
		}

	*/
	return
}
