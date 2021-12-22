package testing

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"math/big"
	"math/rand"
	"time"

	"github.com/AccumulateNetwork/accumulate/internal/api"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	abci "github.com/tendermint/tendermint/abci/types"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
)

func MustParseUrl(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

// Load
// Generate load in our test.  Create a bunch of transactions, and submit them.
func Load(query *api.Query, Origin ed25519.PrivateKey, walletCount, txCount int) (addrList []string, err error) {

	var wallet []*transactions.WalletEntry

	wallet = append(wallet, NewWalletEntry())                // wallet[0] is where we put 5000 ACME tokens
	wallet[0].Nonce = 1                                      // start the nonce at 1
	wallet[0].PrivateKey = Origin                            // Put the private key for the origin
	wallet[0].Addr = AcmeLiteAddressStdPriv(Origin).String() // Generate the origin address

	for i := 0; i < walletCount; i++ { //                            create a 1000 addresses for lite token chains
		wallet = append(wallet, NewWalletEntry()) // create a new wallet entry
	}

	addrCountMap := make(map[string]int)
	for i := 0; i < txCount; i++ { // Make a bunch of transactions
		if i%200 == 0 {
			err = WaitForTxnBatch(query)
			if err != nil {
				return nil, err
			}
		}
		const origin = 0
		randDest := rand.Int()%(len(wallet)-1) + 1                  // pick a destination address
		addrCountMap[wallet[randDest].Addr]++                       // count the number of deposits to output
		addr := AcmeLiteAddressStdPriv(wallet[randDest].PrivateKey) // Make lite address
		send := new(protocol.SendTokens)                            // Create a send token transaction
		send.AddRecipient(addr, 1000)                               // create the transaction output
		gtx := new(transactions.GenTransaction)                     // wrap in a GenTransaction
		gtx.SigInfo = new(transactions.SignatureInfo)               // Get a Signature Info block
		gtx.Transaction, err = send.MarshalBinary()                 // add  send transaction
		if err != nil {
			return nil, err
		}
		gtx.SigInfo.URL = wallet[origin].Addr           // URL of source
		if err := gtx.SetRoutingChainID(); err != nil { // Routing ChainID is the tx source
			return nil, fmt.Errorf("failed to set routing chain ID: %v", err)
		}

		binaryGtx := gtx.TransactionHash() // Must sign the GenTransaction

		gtx.Signature = append(gtx.Signature, wallet[origin].Sign(binaryGtx))

		if _, err := query.BroadcastTx(gtx, nil); err != nil {
			return nil, fmt.Errorf("failed to send TX: %v", err)
		}
	}

	err = WaitForTxnBatch(query)
	if err != nil {
		return nil, err
	}

	for addr, ct := range addrCountMap {
		addrList = append(addrList, addr)
		_ = ct
		fmt.Printf("%s : %d\n", addr, ct*1000)
	}

	return addrList, nil
}

func BuildTestSynthDepositGenTx() (types.String, ed25519.PrivateKey, *transactions.GenTransaction, error) {
	_, privateKey, _ := ed25519.GenerateKey(nil)
	//set destination url address
	destAddress := types.String(AcmeLiteAddressStdPriv(privateKey).String())

	//create a fake synthetic deposit for faucet.
	deposit := new(protocol.SyntheticDepositTokens)
	deposit.Cause = sha256.Sum256([]byte("fake txid"))
	deposit.Token = protocol.ACME
	deposit.Amount = *new(big.Int).SetUint64(5e4 * protocol.AcmePrecision)
	// deposit := synthetic.NewTokenTransactionDeposit(txid[:], adiSponsor, destAddress)
	// amtToDeposit := int64(50000)                             //deposit 50k tokens
	// deposit.DepositAmount.SetInt64(amtToDeposit * protocol.AcmePrecision) // assume 8 decimal places
	// deposit.TokenUrl = tokenUrl

	depData, err := deposit.MarshalBinary()
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to marshal deposit: %v", err)
	}

	gtx := new(transactions.GenTransaction)
	gtx.SigInfo = new(transactions.SignatureInfo)
	gtx.Transaction = depData
	gtx.SigInfo.URL = *destAddress.AsString()
	gtx.ChainID = types.GetChainIdFromChainPath(destAddress.AsString())[:]
	gtx.Routing = types.GetAddressFromIdentity(destAddress.AsString())

	ed := new(transactions.ED25519Sig)
	gtx.SigInfo.Nonce = 1
	ed.PublicKey = privateKey[32:]
	err = ed.Sign(gtx.SigInfo.Nonce, privateKey, gtx.TransactionHash())
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to sign TX: %v", err)
	}

	gtx.Signature = append(gtx.Signature, ed)

	return destAddress, privateKey, gtx, nil
}

func BuildTestTokenTxGenTx(sponsor ed25519.PrivateKey, destAddr string, amount uint64) (*transactions.GenTransaction, error) {
	//use the public key of the bvc to make a sponsor address (this doesn't really matter right now, but need something so Identity of the BVC is good)
	from := types.String(AcmeLiteAddressStdPriv(sponsor).String())

	u, err := url.Parse(destAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}

	send := protocol.SendTokens{}
	send.AddRecipient(u, amount)

	txData, err := send.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal token tx: %v", err)
	}

	gtx := new(transactions.GenTransaction)
	gtx.SigInfo = new(transactions.SignatureInfo)
	gtx.Transaction = txData
	gtx.SigInfo.URL = string(from)
	gtx.ChainID = types.GetChainIdFromChainPath(from.AsString())[:]
	gtx.Routing = types.GetAddressFromIdentity(from.AsString())

	ed := new(transactions.ED25519Sig)
	gtx.SigInfo.Nonce = 1
	ed.PublicKey = sponsor[32:]
	err = ed.Sign(gtx.SigInfo.Nonce, sponsor, gtx.TransactionHash())
	if err != nil {
		return nil, fmt.Errorf("failed to sign TX: %v", err)
	}

	gtx.Signature = append(gtx.Signature, ed)

	return gtx, nil
}

func RunLoadTest(query *api.Query, walletCount, txCount int) (addrList []string, err error) {
	destAddress, privateKey, gtx, err := BuildTestSynthDepositGenTx()
	if err != nil {
		return nil, err
	}

	adiSponsor := gtx.SigInfo.URL

	_, err = query.BroadcastTx(gtx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to send TX: %v", err)
	}
	err = WaitForTxnBatch(query)
	if err != nil {
		return nil, err
	}

	addresses, err := Load(query, privateKey, walletCount, txCount)
	if err != nil {
		return nil, err
	}

	addrList = append(addrList, adiSponsor)
	addrList = append(addrList, *destAddress.AsString())
	addrList = append(addrList, addresses...)
	return addrList, nil
}

func WaitForTxnBatch(query *api.Query) error {
	bs := <-query.BatchSend()
	for _, s := range bs.Status {
		if s.Err != nil {
			return s.Err
		}
		for _, r := range s.Returns {
			r := r.(*coretypes.ResultBroadcastTx)
			if r.Code != 0 {
				return fmt.Errorf("got code %d", r.Code)
			}
			err := WaitForTxHashV1(query, uint64(s.NetworkId), r.Hash)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func SendTxSync(query *api.Query, tx *transactions.GenTransaction) error {
	done := make(chan abci.TxResult)
	_, err := query.BroadcastTx(tx, done)
	if err != nil {
		return fmt.Errorf("failed to send TX: %v", err)
	}
	<-query.BatchSend()

	select {
	case txr := <-done:
		if txr.Result.Code != 0 {
			return fmt.Errorf(txr.Result.Log)
		}

		fmt.Printf("TX %X succeeded\n", sha256.Sum256(txr.Tx))
		return nil

	case <-time.After(1 * time.Minute):
		return fmt.Errorf("timeout while waiting for TX response")
	}
}
