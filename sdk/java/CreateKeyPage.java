package io.accumulatenetwork.accumulate;

public class CreateKeyPage {
	public KeySpecParams[] keys;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.keys == null || this.keys.length == 0)) {
            data = Marshaller.writeValue(data, 2, v.keys);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}