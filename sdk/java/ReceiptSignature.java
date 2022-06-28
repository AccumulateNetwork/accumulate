package io.accumulatenetwork.accumulate;

public class ReceiptSignature {
	public Url sourceNetwork;
	public managed.Receipt proof;
	public byte[] transactionHash;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.sourceNetwork == null)) {
            data = Marshaller.writeUrl(data, 2, this.sourceNetwork);
        }
        if (!(this.proof == null)) {
            data = Marshaller.writeValue(data, 3, v.proof);
        }
        if (!(this.transactionHash == null || this.transactionHash.length == 0)) {
            data = Marshaller.writeHash(data, 4, this.transactionHash);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}