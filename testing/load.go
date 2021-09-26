package testing

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"math/rand"

	"github.com/AccumulateNetwork/accumulated/types"
	anon "github.com/AccumulateNetwork/accumulated/types/anonaddress"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
)

type BroadcastTx func(*transactions.GenTransaction) (*ctypes.ResultBroadcastTx, error)

// Load creates numWallets wallets and sends numTx transactions. For each
// transaction, a random recipient is picked.
func Load(sendTx BroadcastTx, key ed25519.PrivateKey, numWallets, numTx int) (map[string]int, error) {
	// Originating wallet
	origin := new(transactions.WalletEntry)
	origin.Nonce = 1
	origin.PrivateKey = key
	origin.Addr = anon.GenerateAcmeAddress(key[32:])

	// Recipient recipients
	recipients := make([]*transactions.WalletEntry, numWallets)
	for i := range recipients {
		recipients[i] = transactions.NewWalletEntry()
	}

	// Generate and send transactions, tracking how many are set to each recipient
	count := map[string]int{}
	for i := 0; i < numTx; i++ {
		randDest := rand.Intn(len(recipients))                                    // Pick a destination address
		out := transactions.Output{Dest: recipients[randDest].Addr, Amount: 1000} // Create the transaction output
		count[recipients[randDest].Addr]++                                        // Count the number of deposits to output
		send := transactions.NewTokenSend(origin.Addr, out)                       // Create a send token transaction
		gtx := new(transactions.GenTransaction)                                   // Wrap in a GenTransaction
		gtx.SigInfo = new(transactions.SignatureInfo)                             // Get a Signature Info block
		gtx.Transaction = send.Marshal()                                          // Add send transaction
		gtx.SigInfo.URL = origin.Addr                                             // URL of source
		if err := gtx.SetRoutingChainID(); err != nil {                           // Routing ChainID is the tx source
			return nil, fmt.Errorf("bad url generated: %v", err)
		}

		// Sign it
		binaryGtx := gtx.TransactionHash()
		gtx.Signature = append(gtx.Signature, origin.Sign(binaryGtx))

		// Send it
		if _, err := sendTx(gtx); err != nil {
			return nil, fmt.Errorf("failed to send transaction: %v", err)
		}
	}

	return count, nil
}

func NewAcmeWalletTx(sponsorKey, recipientKey ed25519.PrivateKey, txID []byte, tokenURL string) (*transactions.GenTransaction, error) {
	sponsorAddress := types.String(anon.GenerateAcmeAddress(sponsorKey.Public().(ed25519.PublicKey)))
	recipientAddress := types.String(anon.GenerateAcmeAddress(recipientKey.Public().(ed25519.PublicKey)))

	txIDHash := sha256.Sum256(txID)

	//create a fake synthetic deposit for faucet.
	deposit := synthetic.NewTokenTransactionDeposit(txIDHash[:], &sponsorAddress, &recipientAddress)
	amtToDeposit := int64(50000)                             //deposit 50k tokens
	deposit.DepositAmount.SetInt64(amtToDeposit * 100000000) // assume 8 decimal places
	deposit.TokenUrl = types.String(tokenURL)

	depData, err := deposit.MarshalBinary()
	if err != nil {
		return nil, err
	}

	gtx := new(transactions.GenTransaction)
	gtx.SigInfo = new(transactions.SignatureInfo)
	gtx.Transaction = depData
	gtx.SigInfo.URL = *recipientAddress.AsString()
	gtx.ChainID = types.GetChainIdFromChainPath(recipientAddress.AsString())[:]
	gtx.Routing = types.GetAddressFromIdentity(recipientAddress.AsString())

	ed := new(transactions.ED25519Sig)
	gtx.SigInfo.Nonce = 1
	ed.PublicKey = recipientKey.Public().(ed25519.PublicKey)
	err = ed.Sign(gtx.SigInfo.Nonce, recipientKey, gtx.TransactionHash())
	if err != nil {
		return nil, err
	}

	gtx.Signature = append(gtx.Signature, ed)
	return gtx, nil
}
