package io.accumulatenetwork.accumulate;

public class UpdateKey {
	public byte[] newKeyHash;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.newKeyHash == null || this.newKeyHash.length == 0)) {
            data = Marshaller.writeBytes(data, 2, this.newKeyHash);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}