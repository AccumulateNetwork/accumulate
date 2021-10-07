package chain

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"math/big"

	"github.com/AccumulateNetwork/accumulated/types"
	types2 "github.com/AccumulateNetwork/accumulated/types/anonaddress"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
)

type AnonToken struct {
	currentBalanceState map[types.Bytes32]*tokenAccountTx
	currentChainState   map[types.Bytes32]*state.Chain
}

type tokenAccountTx struct {
	account *state.TokenAccount
	txHash  []*types.Bytes32
}

func (*AnonToken) createChain() types.TxType { return types.TxTypeSyntheticTokenDeposit }

func (*AnonToken) updateChain() types.ChainType { return types.ChainTypeAnonTokenAccount }

func (c *AnonToken) BeginBlock() {
	c.currentBalanceState = map[types.Bytes32]*tokenAccountTx{}
	c.currentChainState = map[types.Bytes32]*state.Chain{}
}

func (c *AnonToken) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	return nil
}

func (c *AnonToken) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	switch tx.TransactionType() {
	case uint64(types.TxTypeSyntheticTokenDeposit):
		return c.deposit(st, tx)
	case uint64(types.TxTypeTokenTx):
		return c.sendToken(st, tx)
	default:
		return nil, fmt.Errorf("invalid instruction %d for anonymous token", tx.TransactionType())
	}
}

func (c *AnonToken) deposit(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	//unmarshal the synthetic transaction based upon submission
	deposit := synthetic.TokenTransactionDeposit{}
	err := deposit.UnmarshalBinary(tx.Transaction)
	if err != nil {
		return nil, err
	}

	if deposit.TokenUrl != "dc/ACME" {
		return nil, fmt.Errorf("only ACME tokens can be sent to anonymous token chains")
	}

	//derive the chain for the token account
	adi, _, err := types.ParseIdentityChainPath(deposit.ToUrl.AsString())
	if err != nil {
		return nil, err
	}

	//make sure this is an anonymous address
	if err = types2.IsAcmeAddress(adi); err != nil {
		//need to return to sender
		return nil, fmt.Errorf("deposit token account does not exist and the deposit address is not an anonymous account, %v", err)
	}

	//now check if the anonymous chain already exists.
	txHash := types.Bytes(tx.TransactionHash()).AsBytes32()

	adiChainId := types.GetChainIdFromChainPath(&adi)

	var account *state.TokenAccount

	// if the identity state is nil, then it means we do not have any anon accts setup yet.
	if st.AdiState == nil {
		//setup an anon token account
		object := new(state.Object)
		st.AdiState = object
		account = state.NewTokenAccount(adi, *deposit.TokenUrl.AsString())
		account.Type = types.ChainTypeAnonTokenAccount
	} else {
		//check to see if we have an anon account
		if st.AdiHeader.Type != types.ChainTypeAnonTokenAccount {
			return nil, fmt.Errorf("adi for an anoymous chain is not an anonymous account")
		}
		//now can unmarshal the account
		err := account.UnmarshalBinary(st.AdiState.Entry)
		if err != nil {
			return nil, fmt.Errorf("error unmarshaling anon state account object, %v", err)
		}
	}

	//all is good, so subtract the balance
	err = account.AddBalance(deposit.DepositAmount.AsBigInt())
	if err != nil {
		return nil, fmt.Errorf("unable to add deposit balance to account")
	}

	//now store the token account reference for quick access within the same block
	taTx := &tokenAccountTx{}
	taTx.account = account
	taTx.txHash = append(taTx.txHash, &txHash)
	c.currentBalanceState[*adiChainId] = taTx

	//create a transaction reference chain acme-xxxxx/0, 1, 2, ... n.
	//This will reference the txid to keep the history
	refUrl := fmt.Sprintf("%s/%d", adi, account.TxCount)
	txr := state.NewTxReference(refUrl, txHash[:])
	txrData, err := txr.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("unable to process transaction reference chain %v", err)
	}

	//now create the chain state object.
	txRefChain := new(state.Object)
	txRefChain.Entry = txrData
	txRefChainId := types.GetChainIdFromChainPath(&refUrl)

	//increment the token transaction count
	account.TxCount++

	data, err := account.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("unable to process anon transaction account %v", err)
	}

	st.AdiState.Entry = data

	//if we get here with no errors store the states
	st.DB.AddStateEntry(adiChainId, &txHash, st.AdiState)
	st.DB.AddStateEntry(txRefChainId, &txHash, txRefChain)

	return new(DeliverTxResult), nil
}

func (v *AnonToken) sendToken(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	res := new(DeliverTxResult)

	if st.AdiState == nil {
		return nil, fmt.Errorf("identity state does not exist for anonymous account")
	}

	//now check to make sure this is really an anon account
	if st.AdiHeader.Type != types.ChainTypeAnonTokenAccount {
		return nil, fmt.Errorf("account adi is not an anonymous account type")
	}

	var err error
	withdrawal := api.TokenTx{}
	err = withdrawal.UnmarshalBinary(tx.Transaction)
	if err != nil {
		return nil, fmt.Errorf("error with send token, %v", err)
	}

	//need to derive chain id for coin type account.
	adi, chain, _ := types.ParseIdentityChainPath(withdrawal.From.AsString())
	if adi != chain {
		return nil, fmt.Errorf("cannot specify sub accounts for anonymous token chains")
	}
	//specify the acme tokenUrl
	acmeTokenUrl := types.String("dc/ACME")

	//this is the actual account url the acme tokens are being sent from
	withdrawal.From = types.UrlChain{types.String(adi)}

	//get the ChainId of the acme account for the anon address.
	accountChainId := types.GetChainIdFromChainPath(withdrawal.From.AsString())

	var tokenAccountState *state.TokenAccount
	var taTx *tokenAccountTx
	if taTx = v.currentBalanceState[*accountChainId]; taTx == nil {
		//because we use a different chain for the anonymous account, we need to fetch it.
		st.ChainId = accountChainId
		st.ChainState, err = st.DB.GetCurrentEntry(accountChainId[:])
		if err != nil {
			return nil, fmt.Errorf("chain state for account not esablished")
		}

		tokenAccountState = new(state.TokenAccount)
		err := tokenAccountState.UnmarshalBinary(st.ChainState.Entry)
		if err != nil {
			return nil, err
		}

		taTx = &tokenAccountTx{}
		v.currentBalanceState[*accountChainId] = taTx
		taTx.account = tokenAccountState
		txHash := new(types.Bytes32)
		copy(txHash[:], tx.TransactionHash())
		taTx.txHash = append(taTx.txHash, txHash)
	}
	tokenAccountState = taTx.account

	//now check to see if we can transact
	//really only need to provide one input...
	//now check to see if the account is good to send tokens from
	amt := types.Amount{}
	txAmt := big.NewInt(0)
	for _, val := range withdrawal.To {
		amt.Add(amt.AsBigInt(), txAmt.SetUint64(val.Amount))
	}

	if !tokenAccountState.CanTransact(amt.AsBigInt()) {
		return nil, fmt.Errorf("insufficient balance")
	}

	//so far, so good.  Now we need to check to make sure the signing address is ok.  maybe look at moving this upstream from here.
	//if we get to this function we know we at least have 1 signature
	address := types2.GenerateAcmeAddress(tx.Signature[0].PublicKey)

	//check the addresses to make sure they match
	if address != string(st.AdiHeader.ChainUrl) {
		return nil, fmt.Errorf("invalid address, public key address is %s but account %s ", address, st.AdiHeader.ChainUrl)
	}

	//now build the synthetic transactions.
	txid := types.Bytes(tx.TransactionHash())
	for _, val := range withdrawal.To {
		txAmt.SetUint64(val.Amount)
		//extract the target identity and chain from the url
		destAdi, destChainPath, err := types.ParseIdentityChainPath(val.URL.AsString())
		if err != nil {
			return nil, err
		}
		destUrl := types.String(destChainPath)

		//populate the synthetic transaction, each submission will be signed by BVC leader and dispatched
		gtx := new(transactions.GenTransaction)

		//set the identity chain for the destination
		gtx.Routing = types.GetAddressFromIdentity(&destAdi)
		gtx.ChainID = types.GetChainIdFromChainPath(destUrl.AsString()).Bytes()
		gtx.SigInfo = new(transactions.SignatureInfo)
		gtx.SigInfo.URL = destAdi

		depositTx := synthetic.NewTokenTransactionDeposit(txid[:], &st.AdiHeader.ChainUrl, &destUrl)
		err = depositTx.SetDeposit(&acmeTokenUrl, txAmt)
		if err != nil {
			return nil, fmt.Errorf("unable to set deposit for synthetic token deposit transaction, %v", err)
		}

		gtx.Transaction, err = depositTx.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("unable to marshal synthetic token transaction deposit, %v", err)
		}

		res.AddSyntheticTransaction(gtx)
	}

	err = tokenAccountState.SubBalance(amt.AsBigInt())
	if err != nil {
		return nil, fmt.Errorf("error subtracting balance from account acc://%s, %v", st.AdiHeader.ChainUrl, err)
	}

	txHash := txid.AsBytes32()
	//create a transaction reference chain acme-xxxxx/0, 1, 2, ... n.
	//This will reference the txid to keep the history
	refUrl := fmt.Sprintf("%s/%d", adi, tokenAccountState.TxCount)
	txr := state.NewTxReference(refUrl, txHash[:])
	txrData, err := txr.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("unable to process transaction reference chain %v", err)
	}

	//now create the chain state object.
	txRefChain := new(state.Object)
	txRefChain.Entry = txrData
	txRefChainId := types.GetChainIdFromChainPath(&refUrl)

	//increment the token transaction count
	tokenAccountState.TxCount++

	//want to pass back just an interface rather than marshaled data, for now just
	//do marshaled data
	data, err := tokenAccountState.MarshalBinary()
	if err != nil {
		panic("anon token end block, error marshaling account state.")
	}
	st.AdiState.Entry = data

	//now update the state
	st.DB.AddStateEntry(accountChainId, &txHash, st.AdiState)
	st.DB.AddStateEntry(txRefChainId, &txHash, txRefChain)

	return res, nil
}
