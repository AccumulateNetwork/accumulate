package state

//
//{
//"type": "ACC-0",
//"supply": 10000000,
//"precision": 5,
//"symbol": "EXT",
//"metadata": {"custom-field": "example"}
//}
//this defines a token chain.  the coinbase is the account that is setup that receives, either
//total initial supply, or in the case of unlimited supply, will receive all tokens that are created which
//can be dispatched elsewhere.
type TokenIssuance struct {
	tokentype string //ACC-0 aka FAT-0
	supply    uint64
	precision int8
	symbol    string
	coinbase  string
	metadata  string //don't need here
}
