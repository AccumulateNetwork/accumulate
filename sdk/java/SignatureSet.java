package io.accumulatenetwork.accumulate;

public class SignatureSet {
	public VoteType vote;
	public Url signer;
	public byte[] transactionHash;
	public Signature[] signatures;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.vote == VoteType.Unknown)) {
            data = Marshaller.writeValue(data, 2, v.vote);
        }
        if (!(this.signer == null)) {
            data = Marshaller.writeUrl(data, 3, this.signer);
        }
        if (!(this.transactionHash == null || this.transactionHash.length == 0)) {
            data = Marshaller.writeHash(data, 4, this.transactionHash);
        }
        if (!(this.signatures == null || this.signatures.length == 0)) {
            data = Marshaller.writeValue(data, 5, v.signatures);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}