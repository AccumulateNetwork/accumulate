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
	"acc://c6a629f9a65bf21159c5dfbffbc868ec3ae61ce4651108ec/ACME", "acc://7be239fa0bd119f5e15c4b736c41b5fc5ef458acf1d4ad64/ACME",
	"acc://d22b50671621a0500a5a25d708cbd9819e5297d0991aca46/ACME", "acc://82da5497fb4275ee49e2e67700e535afdfcbf925ca4feb1f/ACME",
	"acc://cee1044b4352a3c6a56d9437960cc8e7645411815c816260/ACME", "acc://bb410a6f22b0edadd13e403258ae6cec05466d9c2397b814/ACME",
	"acc://86f78d7ba392ce3a6ac69df7dabe8030db167c8948001e87/ACME", "acc://9d7d80d6b1a60fab52c93d66ae7dd5f45b83269666071994/ACME",
	"acc://4f71361678a037965e13a0bd98cf948bbf0a0b4e35c316f3/ACME", "acc://35f7d31bf493d819fbe5e10deba5bf9a63ccf045982b005a/ACME",
	"acc://011fec688377975b3b16575b688aa6e2d97e9a9d0de31362/ACME", "acc://e036df4addce381ca1e8eb74d827da81392287af2b7c28ed/ACME",
	"acc://f3cc43ea4c805c393694073b5d2c5e3fa5b48ea7f42582c3/ACME", "acc://f63c25c35abef6d4c35499d3e8bc583a823a308109c91505/ACME",
	"acc://7869f11c2d91d77c5197edc1c9b7ea78fad5715d58b92754/ACME", "acc://cd434b0829e83755a959c2443a9f624e822ab720aa938d78/ACME",
	"acc://af42e1bfe94e3a2a9087433afae252f1cb32dd743140a74e/ACME", "acc://33869bf9796059b621b6c88c0293d098e0d865357dec1d1b/ACME",
	"acc://3a72275f53e257422e43c9378df590bc44646c52986b714e/ACME", "acc://dfdca679ecf815bae006aa81d8df5e81185b139ea01eebcf/ACME",
	"acc://ff19021a2bebc919ee4a58295bb6dc676181ba87f9093fc0/ACME", "acc://8471b2be3668b7567ea25056c11916300d62acf038fe0025/ACME",
	"acc://763ad65ac37d9cab638ea44862eac560823b75d7a38b0d17/ACME", "acc://428939a3d83fa494d93d55a37f6061ed6511f1908a157b39/ACME",
	"acc://b408100a5d3df53e03a6703daba75dc89d22abfdeb27236a/ACME", "acc://cf46fea03b48a98857fbc1b3449675d07e55cfe886aec01b/ACME",
	"acc://17805da27e4c7df4183d7405e4659d8c1cf0abaab8bdd39b/ACME", "acc://9fecc0b01696c30ad4813457cd2fb00179784251cbd57888/ACME",
	"acc://ec1ccf3a5246c35a6c5b03161865ae2bd93f6f8b4076a318/ACME", "acc://9a45a5e4f98753e10cfb3317fbca6acc54e722395ed5004c/ACME",
	"acc://d6f4854eec30c24f8c5b6a8eb88d81bbe049d77f0879a2ea/ACME", "acc://16fbdfbfeb4543e564200c5b53b29a0443493c262a5e2df2/ACME",
	"acc://421cb657e84fe40105cabec37f4dc2de55b00ad7e734492a/ACME", "acc://0f210b23f029aa0f3b8ee7ddfbc8adc33d5ba2f338234c41/ACME",
	"acc://52b12bdf5bb103650125cbac64b4b666f5421eb6b3787833/ACME", "acc://7dc56e702470632ef0677fba0bebd3c09ffe89c673b1887f/ACME",
	"acc://4f5970036ee43ff75b21d01d2dbd3a88fa4930194870e8ea/ACME", "acc://725eb581cbea1b38bd6484e7fe553feb270829d97e69786e/ACME",
	"acc://2aa207d999bd224b07c3b883e0622a4d85cbbb3574f2515c/ACME", "acc://fa2ecc4ba731b213dae1b8230722a8561361d9af682b3788/ACME",
	"acc://361d5804de6935aef26707df4c5f064333a660e3e5000a1c/ACME", "acc://68361ed26d0d16eb1363991f2c9c30e13b3bfff35502dc79/ACME",
	"acc://9ca6287124e1acd0da781abdf64be08858ed69cf1c18989b/ACME", "acc://2960496d75614f8b3a5065beae6277ca1f908dcfe3949d9c/ACME",
	"acc://e0fed9bc6c8ba5c5d52364c2b893a8cad3d7ad80c62d7ab4/ACME", "acc://bc7676356ad4034db4c6f37f4469be86f3f2174c0729419d/ACME",
	"acc://37da0f150dc025820b5bc3d3c47b16a5df23688d4c21b342/ACME", "acc://50c53faeacb9b58bf838b213ba4cb58bd033e2db03e1ca88/ACME",
	"acc://c93a68b14029bab9aa231d4d25b9650f73f03e254b782820/ACME", "acc://48c3a881d487ce088b81fa6a3a0b19b4a92484f5a1e3dd85/ACME",
	"acc://c07169a4562f978c754c073d3804470781ad49546031af0f/ACME", "acc://3db34b012bcc77e9b64a3d15877ce2427d4511e57406dd47/ACME",
	"acc://f2091cc0bbe24db1fa9c2764194e6e62cfc08fb9f6ad90ec/ACME", "acc://c56d15815154d5d84648e19816765fb155242e29c5d38323/ACME",
	"acc://f20662520352f3ed06c037c4ecb1ae59bfab4010c25def06/ACME", "acc://8b296c49c0163db0b27f69c8739a7f6986e69d2906a60df2/ACME",
	"acc://18d512742d2f22ab0b584abebeee20cda97d4a6a04046d48/ACME", "acc://955374ca2b54a3ac6bec54ef6fed99c1c4496ea8b8acf2e9/ACME",
	"acc://682b0df142cb86460da136ed20693d91c338b31f3df6255f/ACME", "acc://89884f8ee879c499325a48e58212985b15f3008cf5a35109/ACME",
	"acc://b01ba6a6c879dc691d814935f07fe75b478bf9f4db38bcc5/ACME", "acc://3fcf629c50d0b91afd050b31a2b16066a16bc3c2c2c31fe6/ACME",
	"acc://e93cce416616ec845ab3d2e5b8b5093258b32f1dbd72dd7e/ACME", "acc://68afb8a6e67fc74efb7e93868523776f97c1cba4871b0692/ACME",
	"acc://0f213ee6d0a2d2567ca419fca0edfdda49e2c8a4cd908aa6/ACME", "acc://f893016e1e6029b8781c4d17fe29846c155a834a4e9d51c3/ACME",
	"acc://afd991b08c51a931a5cec5be3563ae3f7c101a576f1d2d3b/ACME", "acc://f187f5b2c060970c99f8d5f1fc67437a99c613c273afbb22/ACME",
	"acc://4ecce0544e957c832ef7925223cca3b6446153f63281d657/ACME", "acc://0db2fe7564af587e021e3282a0f53693f2c2801b964307b4/ACME",
	"acc://138ac53d4af99b6a9776c6785613c3fe6e32ac755d1ed594/ACME", "acc://892d4249e5fccfd72c70408365a81bce42e1cc095c9a3287/ACME",
	"acc://5cae64aee35b3a70707403f4b4852cb96812f029bc13356f/ACME", "acc://259a14fef511be8cf407f0abefa8e121e52dde475dab9c24/ACME",
	"acc://0cd1622065ab04173764efada20af4e474c13c64adb89748/ACME", "acc://ce8dcb9c82fedccab45f25440d47f99dcf46ffa1f87bed10/ACME",
	"acc://e7637b4607865c033ea9e8f269fca57955e617c332c86092/ACME", "acc://d6d6d86a5263957c5bfe6c84614bda98a0272cc624357ac7/ACME",
	"acc://8b283c79f8483fea32451ffa7623c59c36bb6e11eb1404b8/ACME", "acc://26cd15e2eb3ece6d9b7e4490618dfae668b4b5a1e8a4a1bf/ACME",
	"acc://a1f68dd68b0bc0bb61d854117ff18378624f983bad32342f/ACME", "acc://bdb43c624006f27a366ea39b2678f84ed643abbf0005a950/ACME",
	"acc://4a77dffe96dc806968f6a6f9bb79a35b0bd1b317a6eb1e48/ACME", "acc://491a17e2260fc7503d1c6281d830d81cc75da0ec31495474/ACME",
	"acc://fa63c72213cb7a69a57076798320d21d063cbde003eac0bf/ACME", "acc://7cc1ccb79775109809c72281aae0284119b18352521ec1c1/ACME",
	"acc://3379ee0b1d2e1b38c68d4582aaacb6991c1c00307bb1a55b/ACME", "acc://91e50ea7ac244589114b5f6d486a1aba3326628f6afe70c8/ACME",
	"acc://86d7f8a79491511e453a86c93b01759250b7e0640d41ef78/ACME", "acc://2b71898bbabe7ba7db8c70faada0cf53096772ef9dc83fae/ACME",
	"acc://f5708a8d7690d6efef402d7880a59ea629720a9e0f51e466/ACME", "acc://9df9773b2df8224bf4038993703a004044e6ccc1b3ecf9bd/ACME",
	"acc://6e6f6bf0c11dd51a4706188b1a42aa3a0e682ef27ad2687f/ACME", "acc://9c75a74a7fc9eba1357217ee220f7d6ca010276ad9d04ddb/ACME",
	"acc://484bb3f466ff304731e16844b04db55476a2043ef0dbc4e3/ACME", "acc://d81efe16b1d4fe137c36f7806ba3eef47e7035817326b406/ACME",
	"acc://5a0e20ddcf0653668b4a3cacadc7a437a8e72836b6b46598/ACME", "acc://3a56a9e17017540b95432ae114a9019208c7991a222215e5/ACME",
	"acc://d4a8c6140316e5a90ff63216ac836ef106427dffcef7add3/ACME", "acc://0c42313a1692bb3e6bca75d76d3e6a239d12ff0331f3eb4a/ACME",
}
