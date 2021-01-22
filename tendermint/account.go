package tendermint

import (
	"errors"
	//	"fmt"
	//"github.com/gogo/protobuf/proto"
	"github.com/AccumulateNetwork/accumulated/database"
	pb "github.com/AccumulateNetwork/accumulated/proto"

	"github.com/golang/protobuf/proto"
)

type AccountStateStruct struct {
	PublicKey         []byte
	MessageCountDown  int32
	MessageAllowance  int32
	LastBlockHeight   int64
	LastAccess        int64
	groups []pb.Account_Group
}

func GetAccount(publicKey []byte) (Account pb.Account, err error){

	var accountBytes []byte
	accountBytes, err = database.AccountsDB.Get(publicKey)
	if accountBytes == nil {
		err = errors.New("Account not found")
		return Account,err
	}

	err = proto.Unmarshal(accountBytes,&Account)
	return Account,err
}



type BVCEntryStruct struct {
	ChainAddress     []byte
	MessageCountDown  int32
	MessageAllowance  int32
	LastBlockHeight   int64
	LastAccess        int64
	groups []pb.Account_Group
}