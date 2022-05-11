package factom

import "gitlab.com/accumulatenetwork/accumulate/internal/client"

var factomChainData map[string]*Queue

func accountFromPrivateKey(privateKeyHex string) {

}

func WriteDataToAccumulate(env string) error {
	client, err := client.New(env)
	if err != nil {
		return err
	}
	client.ExecuteWriteDataTo()
}
