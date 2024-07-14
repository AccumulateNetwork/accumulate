const eth = require('@metamask/eth-sig-util');

const privateKey = Buffer.from(process.argv[2], 'hex');
const data = JSON.parse(process.argv[3]);
const sig = eth.signTypedData({ privateKey, data, version: 'V4' });
process.stdout.write(sig);