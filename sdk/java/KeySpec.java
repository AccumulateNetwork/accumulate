package io.accumulatenetwork.accumulate;

public class KeySpec {
	public byte[] publicKeyHash;
	public int lastUsedOn;
	public Url delegate;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.publicKeyHash == null || this.publicKeyHash.length == 0)) {
            data = Marshaller.writeBytes(data, 1, this.publicKeyHash);
        }
        if (!(this.lastUsedOn == 0)) {
            data = Marshaller.writeUint(data, 2, this.lastUsedOn);
        }
        if (!(this.delegate == null)) {
            data = Marshaller.writeUrl(data, 3, this.delegate);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}