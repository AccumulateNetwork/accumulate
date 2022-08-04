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
	_, err := tc.executeTx(t, "data write acc://RedWagon.acme/DataAccount red1 cafef00dbabe8badf00d --sign-data acc://61c185c8c6c929d6ad00aa5529ca880808718258c1bb69df/ACME")
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
	"acc://61c185c8c6c929d6ad00aa5529ca880808718258c1bb69df/ACME", "acc://8861d93730671aad03bc144532d5d0b6d422a8c93cb68940/ACME",
	"acc://dcb37f329ea6423d6677680eed80d0604d2573b899facd49/ACME", "acc://1bd9e22f4e4d67d5c11589f04242ca76ee8e48c723bce32d/ACME",
	"acc://e36ecdd3cb512bf536fdb0ed6b8682a149ce9264a3ef11ba/ACME", "acc://39b4a1f36c33cc50f95f63b5a324d016695d870c86c9533c/ACME",
	"acc://d71701d9e2536bcd5c614892bb55927ada30bb4f0758f482/ACME", "acc://9e099da78c33ce087bd3672de8939a2c3819dfe606ed5923/ACME",
	"acc://cab7d67b2b1318f1dd50841408c438c4739f515d75a51e06/ACME", "acc://1b80560e45f18744cfd0767b8888b6d4fbc89c95e0a74ed0/ACME",
	"acc://0291ff53295a51f62e00520002abec62894a18c01862115f/ACME", "acc://4b4f30304ba9369ff32e484009d192692b718d1f36655a77/ACME",
	"acc://25262d3c2aa83131fd18bd20e2ea1da630a27807154a3bd8/ACME", "acc://d61d05cf84bcd916b0fa525911cb45d1295182ec8de9c4f9/ACME",
	"acc://576b65f321ab26a46ba3f5e78b0819665a456959a6802764/ACME", "acc://bbc6292feb09e57123be56a2a76c39e60f44db3ac0e12839/ACME",
	"acc://4e84021f79c0f069e2e307357ca75c4b20282e8562cecfe3/ACME", "acc://accdb9204789fb79ab9294f606acb0b7500499077d790b71/ACME",
	"acc://65bdd70be6cb4418895267da377a8b0824ffb4faa691c2b9/ACME", "acc://e6fbf4a3276bf783b0e30e9a1d97fbadf2127f0cde9481fc/ACME",
	"acc://a7f07fd521243361b994c18dd0014a36390e64054be32616/ACME", "acc://ffd84fd2702582716e2e70acefc4c79f1530cb44b16bca72/ACME",
	"acc://b19011020941542a1a5351a2ac7b3cf7773f8ed218170292/ACME", "acc://29ee5111f5e77b8d9a7b8ca9b3d497b537b3d4e4545d33f7/ACME",
	"acc://b75fbe19b44c7c74d57fffc419224cdd9db2ffe42de347c0/ACME", "acc://fb2dbadc6d31f089225eb168d01d73575bcb922a808d9014/ACME",
	"acc://c2219ed0565cbaf9a628e71643f004a4797b8bace3c0e30f/ACME", "acc://752821d8bd2c6970ab5b4089aeda799b94f2d333414c4e8e/ACME",
	"acc://d4a398a7c8e5ad4add74acbdda9bc675fad18a823a17326b/ACME", "acc://e368838ab4f6c70317cc9ed080e556cc740edda21ae471b1/ACME",
	"acc://e0bd2d2bd57b11b9e4736d9508c35f434110b6695d204108/ACME", "acc://21d5801a7ddd331758bfbf56e0e4ac096eaaa7a9435cb7e1/ACME",
	"acc://4ef78b0e3c4992b1c952af47652b1c2e186772bb574fca04/ACME", "acc://70fd2d6b05f7d101b398737c403f04774821ac380f8e70c1/ACME",
	"acc://bc1e103de0011a52cf8358d694b8759a424513c5fadb6358/ACME", "acc://590eaacf169cc572ccf21bead8462796bd3d6882554017ad/ACME",
	"acc://481c8de1ea15b9a95020e49c1b6e2ba162de6ce23c0f2a50/ACME", "acc://12118b06233884b2691aa3f9c59108f973a27a05d4966dc5/ACME",
	"acc://0bf4bbb6365d69d46e75fd7b923e959c3cdaea8730ad4f7c/ACME", "acc://55ce46d7ec141f0a81c7dea7081c8a595a3252b5f29de7cf/ACME",
	"acc://81d5b50c75c49119a7e2baa237709d53302615ccefaf7024/ACME", "acc://95fbb7bbe8ab5c2398ef588186492ef73aa0af0d921531c0/ACME",
	"acc://fba07edc16e9ca41fb0d8aaebf9f20298585d089b4281e7e/ACME", "acc://da1a87a5d6f82fbb6923eacd1782b06528d67927ead8cd7f/ACME",
	"acc://5a7b02f50be2babb7fe9d44ad5a0881b210d2da370c18ce3/ACME", "acc://40829a0bf73589142d485dee89490d65c1d179f3d329a8ee/ACME",
	"acc://62542063a8594cd329e268505c70923ec1f4364ad6260791/ACME", "acc://21af3bd907feb5de66dd8550e4bac35b0794113afec63901/ACME",
	"acc://e6b331d79c46204b74f4f6ce5172a09a2b93c0bf687166f1/ACME", "acc://4adcca9e27c3cc7b9a79bab7a064b86f45c1d40eaeb748d1/ACME",
	"acc://39c61623e98189b60e4d706eb6251a178579a67be2703b57/ACME", "acc://511de20a80e3e5da87c5a8183e491d922a5337dd7073aca7/ACME",
	"acc://d7dd8362ad140f7bd8ab645455b0096e057034d011c6dd09/ACME", "acc://e5df66a2e8805b514b1d3d86fa87b2412bec47889811a2e9/ACME",
	"acc://720eca9c2e819d8b00fcf5731a06c7c020cc46156b3cda1e/ACME", "acc://70fad2baa58bd6f6ea4f1a3423b515a855668ebd2374a74f/ACME",
	"acc://ab4f95d8e3b6a43b558e574238869afccb48b6aef41f3cfe/ACME", "acc://c45b3d2db8d122349c01636b086dc312fa880faf4a122057/ACME",
	"acc://b2eb3fb79732ba20509b35161ba10466c9e044bfd7c153c2/ACME", "acc://df9a537276aeb5e5921b439d383103e8ea79390a0d0f1a94/ACME",
	"acc://964497a428bc8619aba26a2f918fe4d1b6ed5289beb42bb2/ACME", "acc://779f4b8e2e813bae87b1e229bbf45eff855a1547e26e8e9d/ACME",
	"acc://ae1b6d8115a794e1b8feb3d63b62e72a67c39f1a3db255e6/ACME", "acc://48f3a9642b7ac26a312ec76f152fb43be5d9b4c24789c174/ACME",
	"acc://9a9a93d50e59b02c556c3eebda7669be178a17498c21146a/ACME", "acc://d7f8f3c664ad0c86d09cc65ab746c25cf7bc8a79d4ee6c0c/ACME",
	"acc://fac0aaa7951dd37c05e65ca03c6eb91e25cecb53c53ed29f/ACME", "acc://7457952e8ff50bae4f3c279dc31e613d01f79fad8d4bb0d1/ACME",
	"acc://1468df97df2fb6e82c236cdcf629f2388e431c7830f7accf/ACME", "acc://8ccc7cf555890a93f6f31150ee488eff8fc56607b353b0eb/ACME",
	"acc://eb4f981ddc5a88a87393900e98d462c325f8610510dca35c/ACME", "acc://636afacd57a66806d919555860ddd660a225001e87bbeb9f/ACME",
	"acc://de445c4640cc924dd47b4489255d1bf016e0bf19115e1e20/ACME", "acc://371836f178de0d357080f9ba6cee214ff17d3de2eb6a40d7/ACME",
	"acc://bbdc90334d68b94847bd017c95ee9f1e7c7191002c47ff6a/ACME", "acc://32ac70c216e330f043bac2b8948f0db9df0ec646406211e9/ACME",
	"acc://120f300d9fbc9224a16bda2694010f166e65fb07a6433798/ACME", "acc://b2764b5bfdb1adba5ccf558425b428514e0abbebfc9ec5b2/ACME",
	"acc://79a03b2c7896b89d9bc836bc31c88ea4a91b8a272f3d3de8/ACME", "acc://c0c53b990c7680cc5b94ee485913c96ac7865f3759248a9a/ACME",
	"acc://63cf48ff66726281fee0386b36c9e957246963acbd49a5b9/ACME", "acc://2aa47bbf71f88c2849e2e0388cf4db3e3723432c2bc90979/ACME",
	"acc://1dfb5a29eb04046cb99be1a3adbe99c6674ceb18e0f4296a/ACME", "acc://ea9d24eaf33931b77fd14bbc00ca6493f908f050a2b82d0d/ACME",
	"acc://584df26ab65bf19d7b142a0161feb4d22ecda335e22e2b09/ACME", "acc://ecaef82a4ee1f9dfa0373f68d602e2afaf82094f2c251705/ACME",
	"acc://31fa868db4fb468358f501ca601c0a579a4530a7fa7aed4f/ACME", "acc://c8c218b6e51c24af6a54c7412afdfc422245d22c95cafae8/ACME",
	"acc://5c2284fe7bf8ded15f38728ec2191ed2d5315563488d053b/ACME", "acc://a56d4dd62ae053797b043b07506ca1153aa87bdd268f1392/ACME",
	"acc://1c3390929f7729a8e8664a4ba50950f900d75345c9530bcb/ACME", "acc://084983a8202101c67193616be62a02effe99c63d8cb21031/ACME",
	"acc://61341fcf189fcbd130e4c8f94a3b158e8370822a64b48db4/ACME", "acc://94f834410fc7b1e396a9264f42308973a298cd12439e9a50/ACME",
	"acc://ce5133baac9b530b47f998c0244b0394efaee495312c4d2f/ACME", "acc://fbd8e461221b468f77247c87e2a78b6f47cce727dd1b0567/ACME",
	"acc://d1f4f1c258ff95b25829c31e89b77d4de882ad2e4102c563/ACME", "acc://b7d5d9f09d5d41f53002a1f17724fa6ba8f63a28e62224d9/ACME",
	"acc://a5253a6a9bf587bdcc6af0a70597e5abb6963dd923818f74/ACME", "acc://dbfb670fdc701001820e7db1f6d3d62d3f53375f3663e1f7/ACME",
}
