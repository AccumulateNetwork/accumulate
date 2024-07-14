const eth = require("@metamask/eth-sig-util");

const privateKey = 'ed8167d5c93177cd9090f2e71e6034a11f8bd6e383f4af28864318481e148581'
const initiator = '0x983efb69d2d8dcfe952850f8469518b93908ee1e0eabd9886f58fee603c4c912'
const publicKey = '0x04be413510976c868b35aa4d6da89ebe0873ad351bcb75438696fd1f5a6a1e4eac2d2ed8c34a547d83f2dc7aca713d2e3e180c70110bb2faf4a12cee752a701706'
// const privateKey = '05ed8167d5c93177cd9090f2e71e6034a11f8bd6e383f4af28864318481e1485'
// const initiator = '0xee42393d7b1fc5013e77f633bd21960fc47b619222164617c03e7946b6a8d786'
// const publicKey = '0x04fb8c16cf3165ac6cd2188aa2014dbc2528e66ce06c881fa1f99fcc6598bf13dc5091400e4e335db65dca5130f037e367b138573f2808665772ffb35208943606'

const data = {
  types: {
    EIP712Domain: [
      { name: "chainId", type: "uint256" },
      { name: "name", type: "string" },
      { name: "version", type: "string" },
    ],
    SignatureMetadata: [
      { name: "publicKey", type: "bytes" },
      { name: "signer", type: "string" },
      { name: "signerVersion", type: "uint64" },
      { name: "timestamp", type: "uint64" },
      { name: "type", type: "string" },
    ],
    TokenRecipient: [
      { name: "amount", type: "uint256" },
      { name: "url", type: "string" },
    ],
    Transaction: [
      { name: "header", type: "TransactionHeader" },
      { name: "body", type: "sendTokens" },
      { name: "signature", type: "SignatureMetadata" },
    ],
    TransactionHeader: [
      { name: "initiator", type: "bytes32" },
      { name: "principal", type: "string" },
    ],
    sendTokens: [
      { name: "to", type: "TokenRecipient[]" },
      { name: "type", type: "string" },
    ],
  },
  primaryType: "Transaction",
  domain: { name: "Accumulate", version: "1.0.0", chainId: 1 },
  message: {
    body: {
      to: [{ amount: "10000000000", url: "acc://other.acme/ACME" }],
      type: "sendTokens",
    },
    header: {
      initiator,
      principal: "acc://adi.acme/ACME",
    },
    signature: {
      publicKey,
      signer: "acc://adi.acme/book/1",
      signerVersion: 1,
      timestamp: 1720564975623,
      type: "eip712TypedData",
    },
  },
};
const sig = eth.TypedDataUtils.eip712Hash(data, "V4").toString("hex");
// const sig = eth.signTypedData({ privateKey, data, version: 'V4' })

process.stdout.write(sig);

/*
process.stderr.write(`!! ${encodeType(primaryType, types)}\n`)
for (const i in encodedValues) {
    const b = abi_utils_1.encode([encodedTypes[i]], [encodedValues[i]])
    process.stderr.write(`!!   ${i == 0 ? ' ' : '+'} ${Buffer.from(b).toString('hex')}\n`)
}
const enc = (0, util_1.arrToBufArr)((0, abi_utils_1.encode)(encodedTypes, encodedValues));
process.stderr.write(`!!   = ${Buffer.from(keccak_1.keccak256(enc)).toString('hex')}\n\n`)
return enc;
*/
