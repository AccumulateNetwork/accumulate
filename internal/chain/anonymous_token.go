package chain

import (
	"errors"
	"fmt"
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

func (*AnonToken) chainType() types.ChainType { return types.ChainTypeAnonTokenAccount }

func (*AnonToken) instruction() types.TxType {
	return types.TxTypeSyntheticTokenDeposit
}

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
	//adiStateData := currentState.IdentityState
	chainState := state.Chain{}

	var txHash types.Bytes32
	copy(txHash[:], tx.TransactionHash())
	adiChainId := types.GetChainIdFromChainPath(&adi)

	// if the identity state is nil, then it means we do not have any anon accts setup yet.
	if st.IdentityState == nil {
		//we'll just create an adi state and set the initial values, and lock it so it cannot be updated.
		chainState.SetHeader(types.String(adi), types.ChainTypeAnonTokenAccount)
		//need to flag this as an anonymous account
		data, err := chainState.MarshalBinary()
		if err != nil {
			// TODO either this is a bug or it needs a comment
			return nil, nil
		}
		c.currentChainState[*types.GetChainIdFromChainPath(&adi)] = &chainState

		st.DB.AddStateEntry(adiChainId, &txHash, data)

	} else {
		err := chainState.UnmarshalBinary(st.IdentityState.Entry)
		if err != nil {
			return nil, err
		}
		if chainState.Type != types.ChainTypeAnonTokenAccount {
			return nil, fmt.Errorf("adi for an anoymous chain is not an anonymous account")
		}
	}

	//Next GetOrCreateTokenAccount
	//the ADI is the Address, so now form the chain from the token type
	url := fmt.Sprintf("%s/%s", adi, deposit.TokenUrl)
	tokenChain := types.GetChainIdFromChainPath(&url)

	//so now look up the token chain from the account
	//The token state *CAN* be nil, if so we need to create it...
	tokenState, err := st.DB.GetCurrentEntry(tokenChain[:])
	if err != nil && !errors.Is(err, state.ErrNotFound) {
		return nil, fmt.Errorf("unable to retrieve token chain for %s: %w", url, err)
	}

	//Unmarshal or create the token account
	account := &state.TokenAccount{}
	if tokenState == nil {
		//we need to create a new state object.
		account = state.NewTokenAccount(url, *deposit.TokenUrl.AsString())
	} else {
		err = account.UnmarshalBinary(tokenState.Entry)
		if err != nil {
			return nil, err
		}
	}

	//all is good, so subtract the balance
	err = account.AddBalance(deposit.DepositAmount.AsBigInt())
	if err != nil {
		return nil, fmt.Errorf("unable to add deposit balance to account")
	}

	//data, err := account.MarshalBinary()

	//add the token account state to the chain.
	//resp.AddStateData(tokenChain, data)

	taTx := &tokenAccountTx{}
	taTx.account = account
	taTx.txHash = append(taTx.txHash, &txHash)
	c.currentBalanceState[*tokenChain] = taTx

	//this will be optimized later.  really only need to record the tx's as a function of chain id then at end block record Tx
	//want to pass back just an interface rather than marshaled data.

	//create a transaction reference chain acme-xxxxx/0, 1, 2, ... n.  This will reference the txid to keep the history

	//increment the token transaction count
	account.TxCount++

	data, err := account.MarshalBinary()
	if err != nil {
		// TODO why is this a panic?
		panic("anon token end block, error marshaling account state.")
	}
	st.DB.AddStateEntry(tokenChain, &txHash, data)

	//if we get here it is successful. Store tx body on main chain, and verification data on pending
	//txPendingState := state.NewPendingTransaction(submission)
	//txState, txPendingState := state.NewTransaction(txPendingState)
	//
	//data, err = txState.MarshalBinary()
	//resp.AddMainChainData(tokenChain, data)

	// since we have a successful transaction, we only need to store the transaction
	// header that we can use to verify what is on the main chain. need to store reason...
	//data, _ = txPendingState.MarshalBinary()
	//resp.AddPendingData(&deposit.Txid, data)

	return new(DeliverTxResult), nil
}

func (v *AnonToken) sendToken(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	res := new(DeliverTxResult)

	if st.IdentityState == nil {
		return nil, fmt.Errorf("identity state does not exist for anonymous account")
	}

	//now check to make sure this is really an anon account
	if st.AdiHeader.Type != types.ChainTypeAnonTokenAccount {
		return nil, fmt.Errorf("account adi is not an anonymous account type")
	}

	var err error
	withdrawal := transactions.TokenSend{}
	_, err = withdrawal.Unmarshal(tx.Transaction)
	if err != nil {
		return nil, fmt.Errorf("error with send token, %v", err)
	}

	//need to derive chain id for coin type account.
	adi, chain, _ := types.ParseIdentityChainPath(&withdrawal.AccountURL)
	if adi != chain {
		return nil, fmt.Errorf("cannot specify sub accounts for anonymous token chains")
	}
	//specify the acme tokenUrl
	acmeTokenUrl := types.String("dc/ACME")

	//this is the actual account url the acme tokens are being sent from
	withdrawal.AccountURL = fmt.Sprintf("%s/%s", adi, acmeTokenUrl)

	//get the ChainId of the acme account for the anon address.
	accountChainId := types.GetChainIdFromChainPath(&withdrawal.AccountURL)

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
	for _, val := range withdrawal.Outputs {
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
	txid := tx.TransactionHash()
	for _, val := range withdrawal.Outputs {
		txAmt.SetUint64(val.Amount)
		//extract the target identity and chain from the url
		destAdi, destChainPath, err := types.ParseIdentityChainPath(&val.Dest)
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
	var txHash types.Bytes32
	copy(txHash[:], txid)

	//taTx := &tokenAccountTx{}
	//taTx.account = tokenAccountState
	//taTx.txHash = &txHash
	//v.currentBalanceState[*accountChainId] = taTx
	//this will be optimized later.  really only need to record the tx's as a function of chain id then at end block record Tx
	//want to pass back just an interface rather than marshaled data.
	data, err := tokenAccountState.MarshalBinary()
	if err != nil {
		panic("anon token end block, error marshaling account state.")
	}
	st.DB.AddStateEntry(accountChainId, &txHash, data)
	//if we get here we were successful so we can put the signature on the pending chain and transaction on the main chain

	//if we get here it is successful. Store tx body on main chain, and verification data on pending
	//txPendingState := state.NewPendingTransaction(submission)
	//txState, txPendingState := state.NewTransaction(txPendingState)
	//data, _ := txState.MarshalBinary()
	//resp.AddMainChainData(accountChainId, data)

	// since we have a successful transaction, we only need to store the transaction
	// header that we can use to verify what is on the main chain. need to store reason...
	// need to redo this based upon updated transactions.SigInfo struct.  Need to store marshaled SigInfo + ED25519
	//data, _ = txPendingState.MarshalBinary()
	//resp.AddPendingData(&txHash, data)

	return res, nil
}
