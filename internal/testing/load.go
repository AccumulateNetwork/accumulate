package testing

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"math/big"
	"math/rand"

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
		send.AddRecipient(addr, big.NewInt(int64(1000)))            // create the transaction output
		gtx := new(transactions.Envelope)                           // wrap in a GenTransaction
		gtx.Transaction = new(transactions.Transaction)             //
		gtx.Transaction.Body, err = send.MarshalBinary()            // add  send transaction
		if err != nil {
			return nil, err
		}
		u, err := url.Parse(wallet[origin].Addr)
		if err != nil {
			return nil, err
		}
		gtx.Transaction.Origin = u // URL of source

		hash := gtx.Transaction.Hash() // Must sign the GenTransaction
		gtx.Signatures = append(gtx.Signatures, wallet[origin].Sign(hash))

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

func BuildTestSynthDepositGenTx() (types.String, ed25519.PrivateKey, *transactions.Envelope, error) {
	_, privateKey, _ := ed25519.GenerateKey(nil)
	//set destination url address
	destAddress := AcmeLiteAddressStdPriv(privateKey)

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

	gtx := new(transactions.Envelope)
	gtx.Transaction = new(transactions.Transaction)
	gtx.Transaction.Body = depData
	gtx.Transaction.Origin = destAddress

	ed := new(transactions.ED25519Sig)
	gtx.Transaction.Nonce = 1
	ed.PublicKey = privateKey[32:]
	err = ed.Sign(gtx.Transaction.Nonce, privateKey, gtx.Transaction.Hash())
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to sign TX: %v", err)
	}

	gtx.Signatures = append(gtx.Signatures, ed)

	return types.String(destAddress.String()), privateKey, gtx, nil
}

func BuildTestTokenTxGenTx(sponsor ed25519.PrivateKey, destAddr string, amount uint64) (*transactions.Envelope, error) {
	//use the public key of the bvc to make a sponsor address (this doesn't really matter right now, but need something so Identity of the BVC is good)
	from := AcmeLiteAddressStdPriv(sponsor)

	u, err := url.Parse(destAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}

	send := protocol.SendTokens{}
	send.AddRecipient(u, big.NewInt(int64(amount)))

	txData, err := send.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal token tx: %v", err)
	}

	gtx := new(transactions.Envelope)
	gtx.Transaction = new(transactions.Transaction)
	gtx.Transaction.Body = txData
	gtx.Transaction.Origin = from

	ed := new(transactions.ED25519Sig)
	gtx.Transaction.Nonce = 1
	ed.PublicKey = sponsor[32:]
	err = ed.Sign(gtx.Transaction.Nonce, sponsor, gtx.Transaction.Hash())
	if err != nil {
		return nil, fmt.Errorf("failed to sign TX: %v", err)
	}

	gtx.Signatures = append(gtx.Signatures, ed)

	return gtx, nil
}

func RunLoadTest(query *api.Query, walletCount, txCount int) (addrList []string, err error) {
	destAddress, privateKey, gtx, err := BuildTestSynthDepositGenTx()
	if err != nil {
		return nil, err
	}

	adiSponsor := gtx.Transaction.Origin.String()

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
			err := WaitForTxHashV1(query, s.Route, r.Hash)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func SendTxSync(query *api.Query, tx *transactions.Envelope) error {
	done := make(chan abci.TxResult)
	_, err := query.BroadcastTx(tx, done)
	if err != nil {
		return fmt.Errorf("failed to send TX: %v", err)
	}
	return WaitForTxnBatch(query)
}
