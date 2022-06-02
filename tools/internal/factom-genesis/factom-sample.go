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
		//"d7445834bff0bc63409ee285781bf554cf59202926e9d475e5c70dc907948a02",
		//"abbc9d836e76dae52c5053637b197934fad063455f1d5cf2a7de2300641ab351",
		//"fdafb989f7e5c58bdd3a0d77d3c55f19815f1a50f5d5af5e6a4a45075e4720e7",
		//"b8fbf68254dab9a9c6d7e6e549c0a8831467866e5b733786b5f1b221ef72c741",
		//"7b73986b984ee6eb3eb0716a213e5afc9dcdb2bbb0feda7e903caaa85dc6b90b",
		//"50489a65a04288162a46ed3cfddb989bf9210079c2de000da44eaaf1c63b9de5",
		//"188978e3c0c7a1838256a774af9d3aebc7441299c39c4118aec48a22eff1b0b6",
		//"c1059b625ccb2837b8daaa918ec27a9b1ced7030f3ba3556afd5f6e87d9984cf",
		//"cd66d4d8142ed9e6352b351bcd68c73b8dd0881ac6d7aa8c410ee964be8d588c",
		//"b45a365537b7c8d2dce0461b991e14cbd5b9decc0801c19a4c9c6c97547bb4fe",
		//"a17cfc2cca0e40b3c71bafe2dccc15bff9edb6b66854c595b80265e9c234d841",
		//"b227b82dccbefe01a7c83df8d2b4a019f4234a0d4c89585ec551a5afa7b43156",
		//"4db3bbf1ff159b9d09fb4c1c4446f9ef06328b82eb269adacdca93619030e79b",
		//"936bcd4f2b9c8f0067e770e61d97086076688a0fa595dafb0819e081c0a10fcd",
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
