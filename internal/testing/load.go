package testing

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/AccumulateNetwork/accumulate/internal/api"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	anon "github.com/AccumulateNetwork/accumulate/types/anonaddress"
	apitypes "github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/synthetic"
	abci "github.com/tendermint/tendermint/abci/types"
	coregrpc "github.com/tendermint/tendermint/rpc/grpc"
)

// Load
// Generate load in our test.  Create a bunch of transactions, and submit them.
func Load(query *api.Query, Origin ed25519.PrivateKey, walletCount, txCount int) (addrList []string, err error) {

	var wallet []*transactions.WalletEntry

	wallet = append(wallet, api.NewWalletEntry())          // wallet[0] is where we put 5000 ACME tokens
	wallet[0].Nonce = 1                                    // start the nonce at 1
	wallet[0].PrivateKey = Origin                          // Put the private key for the origin
	wallet[0].Addr = anon.GenerateAcmeAddress(Origin[32:]) // Generate the origin address

	for i := 0; i < walletCount; i++ { //                            create a 1000 addresses for anonymous token chains
		wallet = append(wallet, api.NewWalletEntry()) // create a new wallet entry
	}

	resultWg := new(sync.WaitGroup)
	resultWg.Add(txCount)
	ch := make(chan abci.TxResult)
	defer close(ch)

	ok := true
	go func() {
		for txr := range ch {
			if txr.Result.Code != 0 {
				fmt.Printf("%v<<\n", txr.Result.Log)
				ok = false
			} else {
				fmt.Printf("TX %X succeeded\n", sha256.Sum256(txr.Tx))
			}

			resultWg.Done()
		}
	}()

	addrCountMap := make(map[string]int)
	for i := 0; i < txCount; i++ { // Make a bunch of transactions
		if i%200 == 0 {
			bs := <-query.BatchSend()
			for _, s := range bs.Status {
				if s.Err != nil {
					fmt.Printf("error received from batch dispatch on network %d, %v\n", s.NetworkId, s.Err)
				}
				for j, t := range s.Returns {
					if resp, ok := t.(coregrpc.ResponseBroadcastTx); ok {
						if len(resp.CheckTx.Log) > 0 {
							fmt.Printf("<%d>%v<<\n", j, resp.CheckTx.Log)
						}
					}
				}
			}
		}
		const origin = 0
		randDest := rand.Int()%(len(wallet)-1) + 1                     // pick a destination address
		addrCountMap[wallet[randDest].Addr]++                          // count the number of deposits to output
		send := apitypes.NewTokenTx(types.String(wallet[origin].Addr)) // Create a send token transaction
		send.AddToAccount(types.String(wallet[randDest].Addr), 1000)   // create the transaction output
		gtx := new(transactions.GenTransaction)                        // wrap in a GenTransaction
		gtx.SigInfo = new(transactions.SignatureInfo)                  // Get a Signature Info block
		gtx.Transaction, err = send.MarshalBinary()                    // add  send transaction
		if err != nil {
			return nil, err
		}
		gtx.SigInfo.URL = wallet[origin].Addr           // URL of source
		if err := gtx.SetRoutingChainID(); err != nil { // Routing ChainID is the tx source
			return nil, fmt.Errorf("failed to set routing chain ID: %v", err)
		}

		binaryGtx := gtx.TransactionHash() // Must sign the GenTransaction

		gtx.Signature = append(gtx.Signature, wallet[origin].Sign(binaryGtx))

		if _, err := query.BroadcastTx(gtx, ch); err != nil {
			return nil, fmt.Errorf("failed to send TX: %v", err)
		}
	}

	bs := <-query.BatchSend()
	for i, s := range bs.Status {
		for _, t := range s.Returns {
			if s.Err != nil {
				ok = false
				fmt.Printf("error received from batch dispatch on network %d, %v\n", s.NetworkId, s.Err)
			}
			if resp, ok := t.(coregrpc.ResponseBroadcastTx); ok {
				if resp.CheckTx.Code > 0 && len(resp.CheckTx.Log) > 0 {
					ok = false
					fmt.Printf("<%d>%v<<\n", i, resp.CheckTx.Log)
				}
			}
		}
	}

	for addr, ct := range addrCountMap {
		addrList = append(addrList, addr)
		_ = ct
		fmt.Printf("%s : %d\n", addr, ct*1000)
	}

	resultWg.Wait()

	if !ok {
		return addrList, fmt.Errorf("one or more transactions failed")
	}
	return addrList, nil
}

func BuildTestSynthDepositGenTx(origin ed25519.PrivateKey) (types.String, ed25519.PrivateKey, *transactions.GenTransaction, error) {
	//use the public key of the bvc to make a sponsor address (this doesn't really matter right now, but need something so Identity of the BVC is good)
	adiSponsor := types.String(anon.GenerateAcmeAddress(origin.Public().(ed25519.PublicKey)))

	_, privateKey, _ := ed25519.GenerateKey(nil)
	//set destination url address
	destAddress := types.String(anon.GenerateAcmeAddress(privateKey.Public().(ed25519.PublicKey)))

	txid := sha256.Sum256([]byte("fake txid"))

	tokenUrl := types.String(protocol.AcmeUrl().String())

	//create a fake synthetic deposit for faucet.
	deposit := synthetic.NewTokenTransactionDeposit(txid[:], adiSponsor, destAddress)
	amtToDeposit := int64(50000)                             //deposit 50k tokens
	deposit.DepositAmount.SetInt64(amtToDeposit * 100000000) // assume 8 decimal places
	deposit.TokenUrl = tokenUrl

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
	from := types.String(anon.GenerateAcmeAddress(sponsor.Public().(ed25519.PublicKey)))

	tokenTx := apitypes.TokenTx{}

	tokenTx.From = types.UrlChain{String: from}
	tokenTx.AddToAccount(types.String(destAddr), amount)

	txData, err := tokenTx.MarshalBinary()
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

func RunLoadTest(query *api.Query, origin ed25519.PrivateKey, walletCount, txCount int) (addrList []string, err error) {
	destAddress, privateKey, gtx, err := BuildTestSynthDepositGenTx(origin)
	if err != nil {
		return nil, err
	}

	adiSponsor := gtx.SigInfo.URL

	err = SendTxSync(query, gtx)
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
