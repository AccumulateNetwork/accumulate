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
	"acc://a147d4b598d5d4343d063c1796e6a0257e3a2867885e2001/ACME", "acc://958fff6dfb5a5703264a74362adb859becf335066f9c1e4c/ACME",
	"acc://99f585c20a3dfb8beb5113fa8bc8d35dd0b627a45979115a/ACME", "acc://b2128d0c1b5a95f15b263ac48389c1e76489ccb10a995ed1/ACME",
	"acc://5cbae53f4cbc3fd8dd876175e806b6c7fe7701641c269617/ACME", "acc://6572bc375040a585b9df3d83adce1cab7484c805b1fe5435/ACME",
	"acc://64fcf5d8ed11bff66ba71df580a4f396fb52e915f98dbf88/ACME", "acc://297800a9497119e7f992ebca9c5833c7dc65fc793ff1b599/ACME",
	"acc://d3a0e2e050d119cb44d824b231dc0cd7c6e71e16d5d45c38/ACME", "acc://ed09d712d88a03bc921e6e21a5a742d2f56f3d25090901f9/ACME",
	"acc://f653afea8b051430e4dab7f572adb723e09b6cdb037946e6/ACME", "acc://8fed4dfeb607fceee3fb59fd87f9d8b2a775e719e512589d/ACME",
	"acc://a982ff159c15d858bb1014d9d530e869ee8739b463dacac1/ACME", "acc://d1a11610cb4fc1e9c9aada3b976df249f4481f06dda13731/ACME",
	"acc://46050c7b3a5bd1fba00185c01fa15f7bfa897bb05aeb857b/ACME", "acc://5d6be1bd9778278edaebf2cf33b19ae4a274531f56243ccb/ACME",
	"acc://1a7b331398daf781a415fd3ee21fb0184daae96832e062cc/ACME", "acc://e81252e025e6fdebcc9ed65128281b765f91126db6870472/ACME",
	"acc://277b01cc7ccc7c217012fee872771afb509890cad058c22b/ACME", "acc://ce60bdcec4548161c012a655aecb43e2ce555abb8acc8eda/ACME",
	"acc://b022a261d5e6b31890e7950c7e281bc31152693b58f2a260/ACME", "acc://e69a22946110329719c7ee2fd71cb12b76069cfe274b59ce/ACME",
	"acc://2a780eb71ab641ab251a87e6abce95b7cb98a54e96473f01/ACME", "acc://fdfe3d09239626fe91397d97495b14355ed5f20cafbea557/ACME",
	"acc://43f1186059c43198deda17be46a1a209bc31e2397e14abe6/ACME", "acc://626514cd6a91e11cfb06e89a09ff917a939b255ab0037a47/ACME",
	"acc://7ae997c87c5898fdf51e6c40a86c1719fe8e355b8f339b80/ACME", "acc://3992ea300f7673233b415fc0704dfe0980f23458655e35f5/ACME",
	"acc://b8d92ca356876139ecf1d29032ed34b08329e0cd78fe268d/ACME", "acc://8e198011a0233383f2ad36ba4e474d02e3c25fc1c06afad6/ACME",
	"acc://c7e22d63dcbd339648080884265e0c3b7d93f3a0ec7042fc/ACME", "acc://46308c5f2260dfc8cd5dd9e41887b41e4d4626eefa7c47b7/ACME",
	"acc://135072a29e0f677f95717326a0bbe8639877526b7e0d510f/ACME", "acc://f5704cb20492d0d4a79c1b56cb69958d9970b276dbed9143/ACME",
	"acc://fbabeeb86d8a43281214fdc28e71a124a81f5cd3e193ab54/ACME", "acc://7ddc6cc4d24c577d967f0611241cc2ca6679cd216a32bd19/ACME",
	"acc://547f67b503b54c702ad0faba01070735661f6d26e7c60a3c/ACME", "acc://3599529910098c68e5f1400ac649667e41b2a7634d5a84a1/ACME",
	"acc://f0d34991ef0cbfd5c8f92ce500b3e66f58c7bfe30eb8d38e/ACME", "acc://b7646309c15e65dd853cd82597077e09e71cfe85f38c98d9/ACME",
	"acc://d557367b3d52a544e391e4fe1ef1c9fbecf626edd6fa67f3/ACME", "acc://b4e64699cc1f29b5e3104864f96f78f89dce539195b0f6dd/ACME",
	"acc://9f60ec82e996e29657d94dda0bc4f7fff0dd2d1198263d01/ACME", "acc://8c06c94f51ca286b1c6e04264bc6c4f986b7a722dbe53494/ACME",
	"acc://8d853ad51b5d9c9f44a36ddd6c4e3871bd47dd1b1cf94060/ACME", "acc://7816f9937dd3f4fdd813861717a624a0e84a500b9025075e/ACME",
	"acc://73ff0a221bcdb0c3b05a28ac6d9739d772cfb7cf28778608/ACME", "acc://d2f61bda13d50e6f902420df4b3d3010d1b33db178966922/ACME",
	"acc://0b9730b16431146daa04e8c86b28799cee7c81d593914a47/ACME", "acc://76e25883a0e7267fede8c25f783f39c06e1a58d09a13464b/ACME",
	"acc://00dd38eddf6fe10042b86ec95cff843330c9b03bd591b1b7/ACME", "acc://326f7fb39b69517c513cee7aaa09b810a8bd79b4b06e9190/ACME",
	"acc://13c8631dbc301dd8ca284a764e660b9e3ced17c1bf42a06d/ACME", "acc://d1da850ac015bd6c5dd6281e37b617b66f90eb0a41b898eb/ACME",
	"acc://1276818d8c397db3475aad7f3f908b40375f51788e75aa36/ACME", "acc://bd974cba60cd5865a03a12c441b6505840f85692b1a5c1e5/ACME",
	"acc://c5bad7647ad0c94500a795df67ae8e472b51e5e70c919210/ACME", "acc://bf5a08488f9a7f56f1ff7c74f74a3e3989f08c8ca7cb9bc7/ACME",
	"acc://2fd4001fcb7ec4c6fa78598e3fc113d97bb43fc28923ca02/ACME", "acc://6bb36e71a49b252adf4574977dcd896cde8caa616d63980e/ACME",
	"acc://d79cfe2e21233996fe47bc7ddf75287477c0ffd5a80cc7ea/ACME", "acc://73174446765a0c01fea92ef6d610ef6221ebec4c858e749e/ACME",
	"acc://bc45e6814c1039ea8b6303171f5b38faf81dfb216ac39bc5/ACME", "acc://9fe8c3975582d781584d33af63712ff04c77da75461b4fb1/ACME",
	"acc://939e1b68b2295ec7531a766abc980627d707f06d795aac87/ACME", "acc://b7444edd526444c6a1e7c4ec19b752b94b9aed5e17f6a0ec/ACME",
	"acc://1a077ea8e3f6047f754b968894bfeb1131758cc3ac83bd9e/ACME", "acc://f64ba1af964c949273179d6dc041f6b67f440c4acb45d1fe/ACME",
	"acc://35b4f99f3fcd755013addbe6c58837d7662c8cb07c58796b/ACME", "acc://4edde51971edbb3cdb6011283dfe56c575878be52954accb/ACME",
	"acc://65858b71c5a22f33dd6778a1a5f7c929fb49016b831a7189/ACME", "acc://9fb6c3cb47c21546938af79a315cc4d0cd595fef684927b5/ACME",
	"acc://09dcf5ecd79491081bdb00f12205adfcac849a9e88234fe8/ACME", "acc://c39595e17d691ac41ee8a584539b576ec26471d05bdc1215/ACME",
	"acc://b3c53d66206cd058a69289402728197e5ee37e84e32942a9/ACME", "acc://4d397a0edcb40c82ba3d2f398507c3c70a6e76bf9f9b1caa/ACME",
	"acc://c53a7a1a23f83d3e1b56d2241e62eeee60900b7acf643778/ACME", "acc://09b0505976689382cd7f2f3b7a70c9451e0acb83ab71d038/ACME",
	"acc://e0527167bb80715d8b2f9e03f75ddbb881a17a76534b7e42/ACME", "acc://fdcbfb569fc2e0a0f53f56bb738aa665e334e3eee4e52e93/ACME",
	"acc://1e7c8081c550a3a3e742efde2ecf9c3e42a264c4f4a45d66/ACME", "acc://4b55c29f4d9327fcd125ef1bfa973ca6bda0b8fa93f8dbf9/ACME",
	"acc://4c3db5b6a09046ee50b6e69c056706c479e145bb470d1e9b/ACME", "acc://11e68426d049d1030c0779d3f3309b7864b574656c9c7b5c/ACME",
	"acc://760a387937b714370020aee74ba229dc27fc18c3eb17f28f/ACME", "acc://b99c977865fe15a54d4e677cff0289135d1dfacc52aa5e99/ACME",
	"acc://9229b64c87a1e711121c23e1ff18e4f93f0dc0e18500aef3/ACME", "acc://8e044c968aeae35946c7848e343545be9a7712ee7be8a8b7/ACME",
	"acc://0b5b72161c4a25d4ddbe00ed35aeb34ac221c6f9cd704e4e/ACME", "acc://38929b70e299e1ca042e0853b9ff60b6a8e6d811fa2a5cf7/ACME",
	"acc://e944e840d4b22f5f42966a45a8ce6dfab2cf1ef9386220a7/ACME", "acc://f87570c49970980c338c75fb8dfed452d44aef0fdb6e2be9/ACME",
	"acc://347e491da508f1ef101b39b4dcc0dac002a4efa24311452b/ACME", "acc://9ff915a5daed3d79f28c44092a9948fa923a74424a072a13/ACME",
	"acc://e2ee1dc5572bd02986c13ef699d9040f8661bee58c7223ba/ACME", "acc://c5068fcd38b8fe069765cc253b3205303c87391308664761/ACME",
	"acc://a17fb9ff1685e4fcd1e5ec751348c6900fe2df6d0731781c/ACME", "acc://08440ab2a3a20c076fa17ef6386c02b03f791ab217adf218/ACME",
	"acc://c61010f78f402e990f8c242805d1fa9f148ce7896c992302/ACME", "acc://c375e6242ab1f2982fa9ff5893bb03d75bb266e6e164ae51/ACME",
}
