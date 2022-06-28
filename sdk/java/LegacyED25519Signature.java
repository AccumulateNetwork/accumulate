package io.accumulatenetwork.accumulate;

public class LegacyED25519Signature {
	public int timestamp;
	public byte[] publicKey;
	public byte[] signature;
	public Url signer;
	public int signerVersion;
	public VoteType vote;
	public byte[] transactionHash;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.timestamp == 0)) {
            data = Marshaller.writeUint(data, 2, this.timestamp);
        }
        if (!(this.publicKey == null || this.publicKey.length == 0)) {
            data = Marshaller.writeBytes(data, 3, this.publicKey);
        }
        if (!(this.signature == null || this.signature.length == 0)) {
            data = Marshaller.writeBytes(data, 4, this.signature);
        }
        if (!(this.signer == null)) {
            data = Marshaller.writeUrl(data, 5, this.signer);
        }
        if (!(this.signerVersion == 0)) {
            data = Marshaller.writeUint(data, 6, this.signerVersion);
        }
        if (!(this.vote == VoteType.Unknown)) {
            data = Marshaller.writeValue(data, 7, v.vote);
        }
        if (!(this.transactionHash == null || this.transactionHash.length == 0)) {
            data = Marshaller.writeHash(data, 8, this.transactionHash);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}