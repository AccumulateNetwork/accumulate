package cmd

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	testMatrix.addTest(testCase2_8)
	testMatrix.addTest(testCase2_9a)
	testMatrix.addTest(testCase2_9b)
	testMatrix.addTest(testCase2_9c)
}

//testCase2_8
//Create an adi data account
func testCase2_8(t *testing.T, tc *testCmd) {

	_, err := tc.executeTx(t, "account create data acc://RedWagon.acme red1 acc://RedWagon.acme/DataAccount")
	require.NoError(t, err)

	//if this doesn't fail, then adi is created
	_, err = tc.execute(t, "adi directory acc://RedWagon.acme 0 10")
	require.NoError(t, err)
}

func testCase2_9a(t *testing.T, tc *testCmd) {

	//pass in some hex encoded stuff 2 ext id's and an encoded data entry
	_, err := tc.executeTx(t, "data write acc://RedWagon.acme/DataAccount red1 badc0de9 deadbeef cafef00dbabe8badf00d")
	require.NoError(t, err)

	//now read back the response
	_, err = tc.execute(t, "data get acc://RedWagon.acme/DataAccount")
	require.NoError(t, err)

	//now read it back as a set
	_, err = tc.execute(t, "data get acc://RedWagon.acme/DataAccount 0 1")
	require.NoError(t, err)

	//now read it back as a expanded set
	_, err = tc.execute(t, "data get acc://RedWagon.acme/DataAccount 0 1 expand")
	require.NoError(t, err)
}

func testCase2_9b(t *testing.T, tc *testCmd) {

	//pass in some hex encoded stuff 2 ext id's and an encoded data entry
	r, err := tc.executeTx(t, "account create data --lite acc://RedWagon.acme/DataAccount red1 466163746f6d2050524f 5475746f7269616c")
	require.NoError(t, err)

	aldr := ActionLiteDataResponse{}
	require.NoError(t, json.Unmarshal([]byte(r), &aldr))
	//now read back the response
	commandLine := fmt.Sprintf("data get %s", aldr.AccountUrl)
	_, err = tc.execute(t, commandLine)
	require.NoError(t, err)

	//now read it back as a set
	_, err = tc.execute(t, fmt.Sprintf("%s 0 1", commandLine))
	require.NoError(t, err)

	//pass in some hex encoded stuff 2 ext id's and an encoded data entry
	commandLine = fmt.Sprintf("data write-to acc://RedWagon.acme/DataAccount red1 %s 0badc0de", aldr.AccountUrl)
	_, err = tc.executeTx(t, commandLine)
	require.NoError(t, err)

	//now read it back as a expanded set
	_, err = tc.execute(t, fmt.Sprintf("%s 0 1 expand", commandLine))
	require.NoError(t, err)
}

func testCase2_9c(t *testing.T, tc *testCmd) {

	//pass in some hex encoded stuff 2 ext id's and an encoded data entry
	_, err := tc.executeTx(t, "data write acc://RedWagon.acme/DataAccount red1 cafef00dbabe8badf00d --sign-data "+liteAccounts[0])
	require.NoError(t, err)

	//now read back the response
	r, err := tc.execute(t, "data get acc://RedWagon.acme/DataAccount")
	require.NoError(t, err)

	var res QueryResponse
	err = json.Unmarshal([]byte(r), &res)
	require.NoError(t, err)
	wd := protocol.WriteData{}
	err = Remarshal(res.Data, &wd)
	require.NoError(t, err)

	if len(wd.Entry.GetData()) != 2 {
		t.Fatal("expected 2 data elements")
	}
	sig, err := protocol.UnmarshalKeySignature(wd.Entry.GetData()[0])
	require.NoError(t, err)

	h := sha256.Sum256(wd.Entry.GetData()[1])
	require.True(t, sig.Verify(nil, h[:]), "invalid signature for signed data")
}

//liteAccounts is the predictable test accounts for the unit tests.
var liteAccounts = []string{
	"acc://8fab12108e2dc0da173c20bc6d240009b5d861255afa6664/ACME", "acc://374595c8853669a12474785cdefc1fae423ff799bdd70e12/ACME",
	"acc://263f45dc2d997befc38390d5bc17c12c3045422cc0c77d8e/ACME", "acc://f4e17f9763cc8477ac91b5c1f4f60373bc5885beeb0319ec/ACME",
	"acc://0ad7b1523c88bb4d3f692b9cbc3d0e72530a6c5ec1aac030/ACME", "acc://82369415bd38ea008817dcc92254a0eac19da2135ec8fbcb/ACME",
	"acc://6625c5f2aedae2576ba18355e033a0bd7c00cb076c6ae0e0/ACME", "acc://7d808812cc9bc09cee8455705f63ad101ddf84c72b379351/ACME",
	"acc://72b9d4747fc7e74b0c86e6979a3304ce35a6e6422ad45119/ACME", "acc://d25c60ed9d9a0cf759111bc1972231117eab439b013caad3/ACME",
	"acc://5c87fd16f3d5646e5057888c488d8c31366ed4ee4be9fcab/ACME", "acc://e7d9631a443edb34cb99716bd13c50e6f0edb9a263695e02/ACME",
	"acc://64d5b5b3a9965e68726b6e7be5b307ec727dd41d3ef78008/ACME", "acc://fd5330e35d4de0ff19d28c1476458747056a35bc11689d0a/ACME",
	"acc://dd6acf914a44289a1273f87ef418afbdfa09e7250b7e272a/ACME", "acc://0b104d64c758a4a6eb276b0d0c2173f61e8832b101d9afa8/ACME",
	"acc://c18374fe45410a49571a66abcabe4c1f80101946064e3ce8/ACME", "acc://a2553a37c3d2e758d111a760098b708498951f5eab4d2229/ACME",
	"acc://a4a87845bbc9d9ab58c1f46a6fc971f70cbc46f7ca42a309/ACME", "acc://eb25214185b954821d2eef5c5a241272fb5130ef44f1e294/ACME",
	"acc://7c2cff881610420cf060e2a0f964130c22f1891bc839bfd3/ACME", "acc://5f1c03e19fee7510bbb58b258cfc1550f43ec20938365a29/ACME",
	"acc://b16fa206c00f3276cb86ed7d94daeb27edf9e63ecc6dcca4/ACME", "acc://0eab4dfb22660d978aaf7553b7de166fda16c51f9df2cd48/ACME",
	"acc://43e2cf10aec90d48fb6e7ad0f283ee1b536819f7a789cf09/ACME", "acc://af1fbbc57c3daae8725ac11e57cc1b242e89316447d8f772/ACME",
	"acc://25e7559076a7f4b83235a34a7d0767c3c0e81b6cd4e10923/ACME", "acc://a7c2568921fde7ef40260b24ee2766e2ae26cd90cec2df4d/ACME",
	"acc://81554dbc171120534a396a4089bc9aeb613babef56bb5f6c/ACME", "acc://bede86b73fb14ec59afa99103b49c1ee6666327407123df8/ACME",
	"acc://eefc43b6f6c5fd002758c667c0a67d7aa9f6959b515819a9/ACME", "acc://46e3f945cdbb41bd6ca30915b0826cb73a197e9847f35aa7/ACME",
	"acc://a442d5c47c20da68c72435486e2492e8b43f2839a2babdde/ACME", "acc://2728263e4b3c97a7a18e0fb1ee9ca444d3619f944774dc67/ACME",
	"acc://4b7b1ceb4fdf3824101fe95f55e86950a6beaf478850a1a1/ACME", "acc://c247c7c1486d47f62a74f7cd80c2ece01ac5404e41a0d094/ACME",
	"acc://a1b9b51e35358472bf64a64028065b14e7f84367ae8afc9c/ACME", "acc://afae055c7ae05ee9ccffdaaed7bdffe5f6ecde1338cf240a/ACME",
	"acc://5b001095bd0bfdfb3a984826b81286f95ab62e069974e2da/ACME", "acc://d928e856fee8040f5a3dbacb6e10e6a826209069f3c5a5ff/ACME",
	"acc://aa93939f201d7f5377076ee9ed1575d3766833f81867f123/ACME", "acc://ce899858021f6f57b73c8111cb1f8c074bc3d873b6e61a97/ACME",
	"acc://400bdb2722270a63b435b8d10dd4066bf714e065f667860b/ACME", "acc://85364bdbb55dc4d9b1a4711f31e2eaf9c3960019797221a7/ACME",
	"acc://2b73251770d57bae574027b68e193a8ba673d9038e8ff51b/ACME", "acc://1dd2ddc2bc0be9cc5f2957f575a146962b898eab74eb31af/ACME",
	"acc://9a9ed82b6bd2cdabc0681bd3cab302a2c111991fce0d6251/ACME", "acc://6f0d476a2816da69833940ba92280c0fe8afb2d1d7c30a96/ACME",
	"acc://33a1af5bf5e0022e42826de74c136ca5000621a08baba60b/ACME", "acc://e6726591c516b8f6ca63c083a3e8dcefecc846de5badb3bc/ACME",
	"acc://303bd36d56076bc3f66d7275382162fa4f73075b6bbe729f/ACME", "acc://ac5fe9f11dc074324428b66340e4a19b6e554a9dce54cb32/ACME",
	"acc://cbe4b5a8ffb74958033424da79f2c0b8ac7be316b56cffc4/ACME", "acc://4981ba977b024fdaf0778051ce1d0fc1291d002f3a80ce91/ACME",
	"acc://b341fe7297b63b20b4e2c2e3b7206370c918bc808f9383d5/ACME", "acc://90935e02644de51e715195ba3d8a89fa91cc85b267c2fc72/ACME",
	"acc://f99b4c60e81dc2c40d91597219d87b6b67f83a1421a6e0c1/ACME", "acc://2c2fe1d472dc2c97b04ff64e37860b53c7d3c8e826a8695d/ACME",
	"acc://7a18993a6883ecb35861f654061f4b4bf952c8f1a65d5be9/ACME", "acc://8940ffca463a957c81a02bd597c3a7ec7e9cffc0df97fe22/ACME",
	"acc://bdf28dd72bff1d7aa6a9c2b5046870aff031e764d9af4d8f/ACME", "acc://34a137d849d8a1bb33f9f1df52ae16f67fd8b6a22b313963/ACME",
	"acc://fd7f919389e568a94a97ad2dfd8f6b846e9007c025685df7/ACME", "acc://235843c1753593a0cbc4ad6c8d53dd7c6fd8b29c59448f2e/ACME",
	"acc://de95b1699817a24942511f7c46896d1eacebf36f7471e131/ACME", "acc://9d7d267e242cc4b60e474d54de15e4700b4d5adee62629e6/ACME",
	"acc://2075e3a66786fab073c9f575f802183c09798ab6ede75b2f/ACME", "acc://2442c5107ddabc47d03485178144e684831b1be1aaff056e/ACME",
	"acc://3c6c6101f498b6f9b13786100eb979739073e469dbcd24f0/ACME", "acc://c25eb1a80417d78021dcbaaf2bdd1fa5a0dfa34a5945e11d/ACME",
	"acc://c35dea9112d6641c54ada364c8bc82887ab0278825ed8d94/ACME", "acc://f81b42b438bdf1dbfadd320fc6959268dc81b01b713b252f/ACME",
	"acc://e988e63ee3e7a8a2e28304da8d4a4a8587c754181323da6f/ACME", "acc://d86555adf6741d66bb8a10f1c77623b109a160db0121114b/ACME",
	"acc://c4a90a40a91029b7986857554e6399c42d92dac257e3d51a/ACME", "acc://e99725be806a0396a63a1f6f9e0a868a98dfa70a5df00b0e/ACME",
	"acc://1fe69af299702b8eaf39963bcbd6fe86738c46e562209e11/ACME", "acc://88895da3a169653dcb4ad89025389a7ab1990597bed67727/ACME",
	"acc://1ba67bff0e5dffe33b29d08c0e8398956027c1080d7a32d2/ACME", "acc://a77ce04f8ebfa632e55579a5df24ca1881e8732103f0c776/ACME",
	"acc://135200a2813d035c9005534d1ea56948bb1130289fffec8b/ACME", "acc://8303c67ce58193511eac7095db1fd9c4698830b8c591c200/ACME",
	"acc://106447691d8186dfeee279f6ad28b26380560d98e92661d9/ACME", "acc://05c375cb1afa6f4fa996c2c29233c0dbf702fba08ce9a3a3/ACME",
	"acc://e3f8b4e0a528ff9189e2145b306064eb83138bce6ad15dde/ACME", "acc://0ef5c5828b95d75b86b1f0d825d8c6d0f4ecd8d929a651b3/ACME",
	"acc://a78628a66d2bfbdfd65ca47ad1c1600fb32f3d2a5b68122e/ACME", "acc://c5ab4fde9363a8d93c090cd70724875f4d4ed62418510d33/ACME",
	"acc://392d042d3e992e7569f812d2989cc722394be9efe51b008b/ACME", "acc://5adfb5c0ff826978a7d5ac7b7fc058dd55dfe8ca406c1849/ACME",
	"acc://30e29e6594448f0ea45c93584c454ff2db27722c3c31168a/ACME", "acc://a65d1535c883195e7fa1a1435c09e2abc785c8a14b384484/ACME",
	"acc://64153ea974d75bcb7b90386e1809fb6afe2aeb31dab8c6f7/ACME", "acc://b12ca41bf8118a4190cf35f52bac859d0f18122b46f20ff6/ACME",
	"acc://63cecdd8afa10e8d31fe0574ed582f60100a142103483eae/ACME", "acc://2b1a089539bd6e6d95b2163b2c768559fcf129b38c84a78f/ACME",
	"acc://16fb151498eacb7f52e4ed8f10498ba3baa7e8bcd0af1f48/ACME", "acc://cca7c26cdf45f1ace947073ae09097c42f3f25df5fff287e/ACME",
	"acc://01324989aa21f5497e40aa08a1163935ff865acf5248f29b/ACME", "acc://ebc97aa3b581ae2ec08ee251c620982b8ed1da6a85af9fbd/ACME",
}
