package io.accumulatenetwork.accumulate;

public class KeySpecParams {
	public byte[] keyHash;
	public Url delegate;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.keyHash == null || this.keyHash.length == 0)) {
            data = Marshaller.writeBytes(data, 1, this.keyHash);
        }
        if (!(this.delegate == null)) {
            data = Marshaller.writeUrl(data, 2, this.delegate);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}