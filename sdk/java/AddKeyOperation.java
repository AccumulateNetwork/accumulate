package io.accumulatenetwork.accumulate;

public class AddKeyOperation {
	public KeySpecParams entry;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.entry == null)) {
            data = Marshaller.writeValue(data, 2, v.entry);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}