package validator

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"net"
	"time"

	"github.com/AccumulateNetwork/accumulated/networks"

	rpchttp "github.com/tendermint/tendermint/rpc/client/http"

	ptypes "github.com/tendermint/tendermint/abci/types"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	pb "github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/spf13/viper"
	tmnet "github.com/tendermint/tendermint/libs/net"
)

// Node implements the general parameters to stimulate the validators, provide synthetic transactions, and issue state changes
type Node struct {
	AccRpcAddr string

	mmDB           state.StateDB
	chainValidator *ValidatorContext
	leader         bool
	key            ed25519.PrivateKey
	rpcClient      *rpchttp.HTTP
	batch          [2]*rpchttp.BatchHTTP
	txBouncer      networks.Bouncer
	height         int64
}

// Initialize will setup the node with the given parameters.  What we really need here are the bouncer, and private key
func (app *Node) Initialize(configFile string, workingDir string, key ed25519.PrivateKey, chainValidator *ValidatorContext) error {
	v := viper.New()
	v.SetConfigFile(configFile)
	v.AddConfigPath(workingDir)
	if err := v.ReadInConfig(); err != nil {

		return fmt.Errorf("viper failed to read config file: %w", err)
	}

	//create a connection to the router.
	app.AccRpcAddr = v.GetString("accumulate.AccRPCAddress")

	networkId := viper.GetString("instrumentation/namespace")
	bvcId := sha256.Sum256([]byte(networkId))
	dbFilename := workingDir + "/" + "valacc.db"
	err := app.mmDB.Open(dbFilename, bvcId[:], true, true)
	if err != nil {
		return fmt.Errorf("failed to open database %s, %v", dbFilename, err)
	}

	laddr := viper.GetString("rpc.laddr")

	app.rpcClient, _ = rpchttp.New(laddr, "/websocket")

	app.chainValidator = chainValidator
	app.leader = false

	app.key = key

	app.batch[0] = app.rpcClient.NewBatch()
	app.batch[1] = app.rpcClient.NewBatch()
	return nil
}

// BeginBlock will set the height, time, and leader (flag if leader for the block)
func (app *Node) BeginBlock(height int64, Time *time.Time, leader bool) error {
	app.leader = leader
	app.height = height
	app.chainValidator.BeginBlock(height, Time)
	return nil
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
func (app *Node) verifyGeneralTransaction(currentState *state.StateEntry, transaction *pb.GenTransaction) error {

	if currentState.IsValid(state.MaskChainState | state.MaskAdiState) {
		//1. verify nonce
		//2. verify public key against state
	}

	//Check to see if transaction is valid. This is expensive, so maybe we should check ADI stuff first.
	if !transaction.ValidateSig() {
		//fmt.Println(fmt.Errorf("invalid signature for transaction %d", transaction.GetTransactionType()))
	}

	return nil
}

// CanTransact will do light validation on the transaction
func (app *Node) CanTransact(transaction *pb.GenTransaction) error {

	//populate the current state from the StateDB
	currentState, _ := app.getCurrentState(transaction.GetChainID())

	if err := app.verifyGeneralTransaction(currentState, transaction); err != nil {
		return fmt.Errorf("cannot proceed with transaction, verification failed, %v", err)
	}

	//run the chain validator...
	app.chainValidator.Check(currentState, transaction)
	return nil
}

// Validate will do a deep validation of the transaction.  This is done by ALL the validators in the network
// transaction []byte will be replaced by transaction RawTransaction
func (app *Node) Validate(transaction *pb.GenTransaction) error {

	currentState, _ := app.getCurrentState(transaction.GetChainID())

	//placeholder for special validation rules for synthetic transactions.
	if transaction.GetTransactionType()&0xFF00 > 0 {
		//need to verify the sender is a legit bvc validator also need the dbvc receipt
		//so if the transaction is a synth tx, then we need to verify the sender is a BVC validator and
		//not an impostor. Need to figure out how to do this. Right now we just assume the synth request
		//sender is legit.
	}

	err := app.verifyGeneralTransaction(currentState, transaction)
	if err != nil {
		return fmt.Errorf("cannot proceed with transaction, verification failed, %v", err)
	}

	//run through the validation routine
	vdata, err := app.chainValidator.Validate(currentState, transaction)

	/// batch any synthetic tx's generated by the validator
	app.processValidatedSubmissionRequest(vdata)

	/// update the state data for the chain.
	if vdata.StateData != nil {
		for k, v := range vdata.StateData {
			header := state.Chain{}
			err := header.UnmarshalBinary(v)
			if err != nil {
				panic("invalid state object after submission processing, should never get here")
			}
			app.mmDB.AddStateEntry(k[:], v)
		}
	}

	if vdata.PendingData != nil {
		for k, v := range vdata.StateData {
			header := state.Chain{}
			err := header.UnmarshalBinary(v)
			if err != nil {
				panic("invalid state object after submission processing, should never get here")
			}
			app.mmDB.AddPendingTx(k[:], v)
		}
	}
	return err
}

// EndBlock will return the merkle DAG root of the current state
func (app *Node) EndBlock() ([]byte, error) {

	mdRoot, numStateChanges, err := app.mmDB.WriteStates(app.chainValidator.GetCurrentHeight())

	if err != nil {
		//shouldn't get here.
		panic(fmt.Errorf("fatal error, block not set, %v", err))
	}

	//if we have no transactions this block then don't publish anything
	if app.leader && numStateChanges > 0 {
		//now we create a synthetic transaction and publish to the directory block validator
		dbvc := ResponseValidateTX{}
		dbvc.Submissions = make([]*pb.GenTransaction, 1)
		dbvc.Submissions[0] = &pb.GenTransaction{}
		dcAdi := "dc"
		dbvc.Submissions[0].ChainID = types.GetChainIdFromChainPath(&dcAdi).Bytes()
		dbvc.Submissions[0].Routing = types.GetAddressFromIdentity(&dcAdi)
		//dbvc.Submissions[0].Transaction = ...

		//broadcast the root
		//app.processValidatedSubmissionRequest(&dbvc)
	}

	//probably should use channels here instead.
	go app.dispatch(int(app.height % 2))

	//app.txBouncer.BatchSend()

	fmt.Printf("DB time %f\n", app.mmDB.TimeBucket)
	app.mmDB.TimeBucket = 0
	return mdRoot, nil
}

// Query will take a query object, process it, and return either the data or error
func (app *Node) Query(q *pb.Query) ([]byte, error) {

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

	// fmt.Printf("Query URI: %s", q.Query)
	return chainState.Entry, nil
}

//processValidatedSubmissionRequest Figure out what to do with the processed validated transaction.  This may include firing off a synthetic TX or simply
//updating the state of the transaction
func (app *Node) processValidatedSubmissionRequest(vdata *ResponseValidateTX) (err error) {
	if vdata == nil {
		return nil
	}

	//need to pass this to a threaded batcher / dispatcher to do both signing and sending of synth tx.  No need to
	//spend valuable time here doing that.
	for _, v := range vdata.Submissions {

		//generate a synthetic tx and send to the router.
		//need to track txid to make sure they get processed....
		if app.leader {
			//we may want to reconsider making this a go call since using grpc could delay things considerably.
			//we only need to make sure it is processed by the next EndBlock so place in pending queue.
			///if we are the leader then we are responsible for dispatching the synth tx.

			dataToSign := v.Transaction
			if dataToSign == nil {
				panic("no synthetic transaction defined.  shouldn't get here.")
			}

			ed := new(pb.ED25519Sig)
			ed.Nonce = 1 //uint64(time.Now().Unix())
			ed.PublicKey = app.key[32:]
			err := ed.Sign(app.key, dataToSign)
			if err != nil {
				panic(fmt.Sprintf("cannot sign synthetic transaction, shoudn't get here, %v", err))
			}

			v.Signature = append(v.Signature, ed)

			deliverRequestTXAsync := new(ptypes.RequestDeliverTx)
			deliverRequestTXAsync.Tx, err = v.Marshal()
			if err != nil {
				return err
			}

			_, err = app.batch[app.height%2].BroadcastTxAsync(context.Background(), deliverRequestTXAsync.Tx)
			if err != nil {
				return err
			}

		}
	}
	return nil
}

func (app *Node) dispatch(batchNum int) {
	if app.batch[batchNum].Count() > 0 {
		app.batch[batchNum].Send(context.Background())
		app.batch[batchNum].Clear()
	}
}

// dialerFunc is a helper function for protobuffers
func dialerFunc(ctx context.Context, addr string) (net.Conn, error) {
	return tmnet.Connect(addr)
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

	tasstatedata, err := tas.MarshalBinary()
	if err != nil {
		panic(err)
	}
	err = app.mmDB.AddStateEntry(identity[:], idStateData)
	if err != nil {
		panic(err)
	}
	err = app.mmDB.AddStateEntry(chainid[:], tasstatedata)
	if err != nil {
		panic(err)
	}
}
