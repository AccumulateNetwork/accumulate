package main

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/tools/internal/factom-genesis"
)

func main() {
	url, _ := factom.AccountFromPrivateKey("G+4fkDMBFaE9Lz/i2sIgiyoRGEgrbjtDdeVn5dhMm/EwGJOPm6m2avDOsNM9VjKosAH93T4e6qkhlYflqheESA==")
	fmt.Println("URL : ", url)
	entries := factom.CurlEntryFromFactom()
	factom.GetDataAndPopulateQueue(entries)
	factom.WriteDataFromQueueToAccumulate()
}
