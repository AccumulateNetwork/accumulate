package validator

import (
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/AccumulateNetwork/accumulated/config"
	"github.com/AccumulateNetwork/accumulated/internal/relay"
	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	pb "github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/spf13/viper"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
)

// Node implements the general parameters to stimulate the validators, provide synthetic transactions, and issue state changes
type Node struct {
	mmDB           state.StateDB     // State database
	chainValidator *ValidatorContext // Chain Validator
	leader         bool
	key            ed25519.PrivateKey
	nonce          uint64

	txBouncer *relay.Relay
	height    int64

	wait      sync.WaitGroup
	chainWait map[uint64]*sync.WaitGroup
	mutex     sync.Mutex
}

// Initialize will setup the node with the given parameters.  What we really need here are the bouncer, and private key
func (app *Node) Initialize(config *config.Config, key ed25519.PrivateKey, chainValidator *ValidatorContext) error {
	networkId := config.Instrumentation.Namespace
	bvcId := sha256.Sum256([]byte(networkId))
	dbFilename := filepath.Join(config.RootDir, "valacc.db")
	err := app.mmDB.Open(dbFilename, bvcId[:], false, true)
	if err != nil {
		return fmt.Errorf("opening database %s: %v", dbFilename, err)
	}

	laddr := viper.GetString("rpc.laddr")

	rpcClient, err := rpchttp.New(laddr, "/websocket")
	if err != nil {
		return fmt.Errorf("creating RPC client: %v", err)
	}

	app.chainValidator = chainValidator
	app.leader = false

	app.key = key

	app.txBouncer = relay.New(rpcClient)
	app.chainValidator.Initialize(nil, &app.mmDB)
	return nil
}

// BeginBlock will set the height, time, and leader (flag if leader for the block)
func (app *Node) BeginBlock(height int64, Time *time.Time, leader bool) error {
	app.leader = leader
	app.height = height
	err := app.chainValidator.BeginBlock(height, Time)
	app.chainWait = make(map[uint64]*sync.WaitGroup)

	return err
}

// getCurrentState will populate the basic state structure with needed information
// for validating transactions.  Some items are allowed to be nil, for
// example, if chains are not created yet.  Based upon what is set is part of the
// validation process.  For example, if ADI state is set, but chain state is not,
// then that limits the transactions for that of creating chains.  If neither chain
// nor adi is set, then it will limit it valid tx's to create anon token account
// or create adi.
func (app *Node) getCurrentState(chainId []byte) (*state.StateEntry, error) {
	currentState := state.NewStateEntry(nil, nil, &app.mmDB)

	currentState.ChainState, _ = app.mmDB.GetCurrentEntry(chainId)

	if currentState.ChainState != nil {
		//we have the chain state, now look up the adi if needed
		chainHeader := state.Chain{}
		err := chainHeader.UnmarshalBinary(currentState.ChainState.Entry)
		if err != nil {
			return currentState, fmt.Errorf("cannot unmarshal chain header, %v", err)
		}

		currentState.ChainHeader = &chainHeader

		//get the chain id of the ADI or Anonymous Chain
		currentState.AdiChain = types.GetIdentityChainFromIdentity(chainHeader.ChainUrl.AsString())
		currentState.IdentityState, _ = app.mmDB.GetCurrentEntry(currentState.AdiChain.Bytes())

		if currentState.IdentityState != nil {
			//Let's get the header for the ADI, it will at least have the nonce which all ADI types will have
			adiHeader := state.Chain{}
			err = adiHeader.UnmarshalBinary(currentState.IdentityState.Entry)
			if err != nil {
				return currentState, fmt.Errorf("cannot unmarshal chain header, %v", err)
			}
			currentState.AdiHeader = &adiHeader
		}

	}

	return currentState, nil
}

// verifyGeneralTransaction will check the nonce (if applicable), check the public key for the transaction,
// and verify the signature of the transaction.
func (app *Node) verifyGeneralTransaction(currentState *state.StateEntry, transaction *transactions.GenTransaction) error {

	if currentState.IsValid(state.MaskChainState | state.MaskAdiState) {
		//1. verify nonce
		//2. verify public key against state
	}

	//Check to see if transaction is valid. This is expensive, so maybe we should check ADI stuff first.
	if !transaction.ValidateSig() {
		return fmt.Errorf("invalid signature for transaction %d", transaction.TransactionType())
	}

	return nil
}

// CanTransact will do light validation on the transaction
func (app *Node) CanTransact(transaction *transactions.GenTransaction) error {
	if err := transaction.SetRoutingChainID(); err != nil {
		return err
	}
	app.mutex.Lock()
	//populate the current state from the StateDB
	currentState, _ := app.getCurrentState(transaction.ChainID)
	app.mutex.Unlock()

	if err := app.verifyGeneralTransaction(currentState, transaction); err != nil {
		return fmt.Errorf("cannot proceed with transaction, verification failed, %v", err)
	}

	//run the chain validator...
	return app.chainValidator.Check(currentState, transaction)
}

func (app *Node) doValidation(transaction *transactions.GenTransaction) error {

	if transaction.Transaction == nil || transaction.SigInfo == nil ||
		len(transaction.ChainID) != 32 {
		return fmt.Errorf("malformd general transaction")
	}

	_ = transaction.TransactionHash()

	app.mutex.Lock()

	var group *sync.WaitGroup
	if group = app.chainWait[transaction.Routing%4]; group == nil {
		group = &sync.WaitGroup{}
		app.chainWait[transaction.Routing%4] = group
	}
	group.Wait()
	group.Add(1)
	app.mutex.Unlock()

	defer func() {
		group.Done()
		app.wait.Done()
	}()

	currentState, _ := app.getCurrentState(transaction.ChainID)

	//placeholder for special validation rules for synthetic transactions.
	if transaction.TransactionType()&0xF0 > 0 {
		//need to verify the sender is a legit bvc validator also need the dbvc receipt
		//so if the transaction is a synth tx, then we need to verify the sender is a BVC validator and
		//not an impostor. Need to figure out how to do this. Right now we just assume the synth request
		//sender is legit.
	}

	err := app.verifyGeneralTransaction(currentState, transaction)
	if err != nil {
		return fmt.Errorf("cannot proceed with transaction, verification failed, %v", err)
	}

	//first configure the pending state which is the basis for the transaction
	txPending := state.NewPendingTransaction(transaction)

	//run through the validation routine
	//todo: vdata should return a list of chainId's the transaction touched.
	vdata, err := app.chainValidator.Validate(currentState, transaction)

	var chainId types.Bytes32
	copy(chainId[:], transaction.ChainID)

	//now check to see if the transaction was accepted
	var txAccepted *state.Transaction
	if err == nil {
		//If we get here, we were successful in validating.  So, we need to split the transaction in 2,
		//he body (i.e. TxAccepted), and the validation material (i.e. TxPending).  The body of the transaction
		//gets put on the main chain, and the validation material gets put on the pending chain which is purged
		//after about 2 weeks
		txAccepted, txPending = state.NewTransaction(txPending)
	}

	//so now we need to store the tx state
	err = app.mmDB.AddPendingTx(&chainId, txPending, txAccepted)

	if err != nil {
		//if we did get an error we just need to return.
		return err
	}

	/// batch any synthetic tx's generated by the validator
	if err := app.processValidatedSubmissionRequest(vdata); err != nil || vdata == nil {
		if vdata == nil {
			err = errors.New("no chain validation")
		}
		return err
	}

	/// update the state data for the chain. <== move to end block!
	//if vdata.StateData != nil {
	//	for k, v := range vdata.StateData {
	//		header := state.Chain{}
	//		err := header.UnmarshalBinary(v)
	//		if err != nil {
	//			panic("invalid state object after submission processing, should never get here")
	//		}
	//		if err := app.mmDB.AddStateEntry(k[:], v); err != nil {
	//			panic("should not error adding state entry")
	//		}
	//	}
	//}

	return nil
}

// Validate will do a deep validation of the transaction.  This is done by ALL the validators in the network
// transaction []byte will be replaced by transaction RawTransaction
func (app *Node) Validate(transaction *transactions.GenTransaction) (err error) {

	app.wait.Add(1)
	err = app.doValidation(transaction)
	//causes mempool overflows. need to investigate more.
	//go app.doValidation(transaction)
	return err
}

// EndBlock will return the merkle DAG root of the current state
func (app *Node) EndBlock() ([]byte, error) {
	app.wait.Wait()
	app.chainValidator.EndBlock([]byte{})
	mdRoot, numStateChanges, err := app.mmDB.WriteStates(app.chainValidator.GetCurrentHeight())

	if err != nil {
		//shouldn't get here.
		panic(fmt.Errorf("fatal error, block not set, %v", err))
	}

	//if we have no transactions this block then don't publish anything
	if app.leader && numStateChanges > 0 {
		//now we create a synthetic transaction and publish to the directory block validator
		dbvc := ResponseValidateTX{}
		dbvc.Submissions = make([]*transactions.GenTransaction, 1)
		dbvc.Submissions[0] = &transactions.GenTransaction{}
		dcAdi := "dc"
		dbvc.Submissions[0].ChainID = types.GetChainIdFromChainPath(&dcAdi).Bytes()
		dbvc.Submissions[0].Routing = types.GetAddressFromIdentity(&dcAdi)
		//dbvc.Submissions[0].Transaction = ...

		//broadcast the root
		//app.processValidatedSubmissionRequest(&dbvc)
	}

	app.txBouncer.BatchSend()

	fmt.Printf("DB time %f\n", app.mmDB.TimeBucket)
	app.mmDB.TimeBucket = 0
	return mdRoot, nil
}

// Query will take a query object, process it, and return either the data or error
func (app *Node) Query(q *pb.Query) (ret []byte, err error) {

	if q.Query != nil {
		tx, pendingTx, _ := app.mmDB.GetTx(q.Query)
		ret = common.SliceBytes(tx)
		ret = append(ret, common.SliceBytes(pendingTx)...)
	} else {
		//extract the state for the chain id
		chainState, err := app.mmDB.GetCurrentEntry(q.ChainId)
		if err != nil {
			return nil, fmt.Errorf("chain id query, %v", err)
		}

		chainHeader := state.Chain{}
		err = chainHeader.UnmarshalBinary(chainState.Entry)
		if err != nil {
			return nil, fmt.Errorf("unable to extract chain header\n")
		}
		ret = chainState.Entry
	}
	// fmt.Printf("Query URI: %s", q.Query)
	return ret, nil
}

//processValidatedSubmissionRequest Figure out what to do with the processed validated transaction.  This may include firing off a synthetic TX or simply
//updating the state of the transaction
func (app *Node) processValidatedSubmissionRequest(vdata *ResponseValidateTX) (err error) {
	if vdata == nil {
		return nil
	}

	//need to pass this to a threaded batcher / dispatcher to do both signing and sending of synth tx.  No need to
	//spend valuable time here doing that.
	for _, gtx := range vdata.Submissions {

		//generate a synthetic tx and send to the router.
		//need to track txid to make sure they get processed....
		if app.leader {
			//we may want to reconsider making this a go call since using grpc could delay things considerably.
			//we only need to make sure it is processed by the next EndBlock so place in pending queue.
			///if we are the leader then we are responsible for dispatching the synth tx.

			dataToSign := gtx.Transaction
			if dataToSign == nil {
				panic("no synthetic transaction defined.  shouldn't get here.")
			}

			if gtx.SigInfo == nil {
				panic("siginfo for synthetic transaction is not set, shouldn't get here")
			}

			ed := new(transactions.ED25519Sig)
			gtx.SigInfo.Nonce = uint64(time.Now().Unix())
			ed.PublicKey = app.key[32:]
			err = ed.Sign(gtx.SigInfo.Nonce, app.key, gtx.TransactionHash())
			if err != nil {
				panic(err)
			}

			gtx.Signature = append(gtx.Signature, ed)

			_, err = app.txBouncer.BatchTx(gtx)

			if err != nil {
				return err
			}

		}
	}
	return nil
}

func (app *Node) createBootstrapAccount() {
	tokenUrl := "wileecoyote/ACME"
	adi, chainPath, err := types.ParseIdentityChainPath(&tokenUrl)
	if err != nil {
		panic(err)
	}

	is := state.NewIdentityState(adi)
	keyHash := sha256.Sum256(app.key)
	_ = is.SetKeyData(state.KeyTypeSha256, keyHash[:])
	idStateData, err := is.MarshalBinary()
	if err != nil {
		panic(err)
	}

	identity := types.GetIdentityChainFromIdentity(&adi)
	chainid := types.GetChainIdFromChainPath(&chainPath)

	ti := api.NewToken(chainPath, "ACME", 8)

	tas := state.NewToken(chainPath)
	tas.Precision = ti.Precision
	tas.Symbol = ti.Symbol
	tas.Meta = ti.Meta

	txid := types.Bytes32(sha256.Sum256([]byte("genesis")))
	tasstatedata, err := tas.MarshalBinary()
	if err != nil {
		panic(err)
	}
	err = app.mmDB.AddStateEntry(identity, &txid, idStateData)
	if err != nil {
		panic(err)
	}
	err = app.mmDB.AddStateEntry(chainid, &txid, tasstatedata)
	if err != nil {
		panic(err)
	}
}
