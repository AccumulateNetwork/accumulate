package factom

import (
	"log"

	f2 "github.com/FactomProject/factom"
)

func EntriesFromFactom() []*f2.Entry {
	//these are the hashes for the first 10 entries of chain df3ade9eec4b08d5379cc64270c30ea7315d8a8a1a69efe2b98a60ecdd69e604
	entryHashes := []string{
		"24674e6bc3094eb773297de955ee095a05830e431da13a37382dcdc89d73c7d7",
		"6503b95dd59ee421ef18f6f221e4e372a99be28aaa73d7572286532d667bfcdd",
		"5f9457e8ad1eb2d7a6f2b640141035e6a1e4389d81ca6e18aab9705a83d42e48",
		"96b2b60a0e026f3aac01e1680b4d4205ec696845b1b18a1ab6340e21835b6cfe",
		"0503fe82359416fc8caecc4a33fbbe94b78f02929e91cbbd022a3c5cab685f6b",
		"a479f8d5d76be64f1a82d0a1f9bdcc2d29ab9507811660e095a8423a515d877a",
		"a952983ddf6331a705b762f30cd01265b23ec5ce98b5398326bf4b14f3a708f6",
		"8832d34336e652082c5f7fa5ce6a3cda2a4aa521eb36b1d115c51a385fa6eb5a",
		"b928de9cd3b3d3fe5748883b59748e0c81f37d89bdbc5438233a20450f9c5c2e",
		"db3bece2fe56b8cac75f36dd519588ab2fee4ed3042b86239c847f16b62e9557",
	}

	retEntries := []*f2.Entry{}
	for _, e := range entryHashes {
		entry, err := f2.GetEntry(e)
		if err != nil {
			log.Fatalf("error retrieving entry %s, %v", e, err)
		}
		retEntries = append(retEntries, entry)
	}

	return retEntries
}
