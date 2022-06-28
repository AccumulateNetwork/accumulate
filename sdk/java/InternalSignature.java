package io.accumulatenetwork.accumulate;

public class InternalSignature {
	public byte[] cause;
	public byte[] transactionHash;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.cause == null || this.cause.length == 0)) {
            data = Marshaller.writeHash(data, 2, this.cause);
        }
        if (!(this.transactionHash == null || this.transactionHash.length == 0)) {
            data = Marshaller.writeHash(data, 3, this.transactionHash);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}