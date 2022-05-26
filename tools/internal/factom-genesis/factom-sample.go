package factom

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

type Entry struct {
	ChainId []byte   `json:"chainid"`
	Content string   `json:"content"`
	ExtIds  [][]byte `json:"extids"`
}

type Response struct {
	JsonRpc string `json:"jsonrpc"`
	Id      int    `json:"id"`
	Result  Entry  `json:"result"`
}

func CurlEntryFromFactom() []*Entry {
	var entries []*Entry
	entryHashes := []string{
		"d7445834bff0bc63409ee285781bf554cf59202926e9d475e5c70dc907948a02",
		"abbc9d836e76dae52c5053637b197934fad063455f1d5cf2a7de2300641ab351",
		"fdafb989f7e5c58bdd3a0d77d3c55f19815f1a50f5d5af5e6a4a45075e4720e7",
		"b8fbf68254dab9a9c6d7e6e549c0a8831467866e5b733786b5f1b221ef72c741",
		"7b73986b984ee6eb3eb0716a213e5afc9dcdb2bbb0feda7e903caaa85dc6b90b",
		"50489a65a04288162a46ed3cfddb989bf9210079c2de000da44eaaf1c63b9de5",
		"188978e3c0c7a1838256a774af9d3aebc7441299c39c4118aec48a22eff1b0b6",
		"c1059b625ccb2837b8daaa918ec27a9b1ced7030f3ba3556afd5f6e87d9984cf",
		"cd66d4d8142ed9e6352b351bcd68c73b8dd0881ac6d7aa8c410ee964be8d588c",
		"b45a365537b7c8d2dce0461b991e14cbd5b9decc0801c19a4c9c6c97547bb4fe",
		"a17cfc2cca0e40b3c71bafe2dccc15bff9edb6b66854c595b80265e9c234d841",
		"b227b82dccbefe01a7c83df8d2b4a019f4234a0d4c89585ec551a5afa7b43156",
		"4db3bbf1ff159b9d09fb4c1c4446f9ef06328b82eb269adacdca93619030e79b",
		"936bcd4f2b9c8f0067e770e61d97086076688a0fa595dafb0819e081c0a10fcd",
	}
	for _, entryHash := range entryHashes {
		data := `{"jsonrpc": "2.0", "id": 0, "method":"entry","params":{"hash":"` + entryHash + ` "}}`
		// data := `{"jsonrpc": "2.0", "id": 0, "method":"entry","params":{"hash":"c1059b625ccb2837b8daaa918ec27a9b1ced7030f3ba3556afd5f6e87d9984cf"}}`
		//curl := exec.Command("curl", "-X", "POST", "--data-binary", data, "-H", "content-type:text/plain;", "http://localhost:8088/v2")
		curl := exec.Command("curl", "-X", "POST", "--data-binary", data, "-H", "content-type:text/plain;", "https://api.factomd.net/v2")
		output, err := curl.Output()
		if err != nil {
			fmt.Println("Error : ", err)
		}
		var res Response
		err = json.Unmarshal(output, &res)
		if err != nil {
			fmt.Println("Error : ", err)
		}
		entries = append(entries, &res.Result)
	}
	return entries
}
