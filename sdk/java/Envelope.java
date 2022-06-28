package io.accumulatenetwork.accumulate;

public class Envelope {
	public Signature[] signatures;
	public byte[] txHash;
	public Transaction[] transaction;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.signatures == null || this.signatures.length == 0)) {
            data = Marshaller.writeValue(data, 1, v.signatures);
        }
        if (!(this.txHash == null || this.txHash.length == 0)) {
            data = Marshaller.writeBytes(data, 2, this.txHash);
        }
        if (!(this.transaction == null || this.transaction.length == 0)) {
            data = Marshaller.writeValue(data, 3, v.transaction);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}