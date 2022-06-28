package io.accumulatenetwork.accumulate;

public class RemoteTransaction {
	public byte[] hash;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.hash == null || this.hash.length == 0)) {
            data = Marshaller.writeHash(data, 2, this.hash);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}