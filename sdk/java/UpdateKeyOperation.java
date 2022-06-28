package io.accumulatenetwork.accumulate;

public class UpdateKeyOperation {
	public KeySpecParams oldEntry;
	public KeySpecParams newEntry;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.oldEntry == null)) {
            data = Marshaller.writeValue(data, 2, v.oldEntry);
        }
        if (!(this.newEntry == null)) {
            data = Marshaller.writeValue(data, 3, v.newEntry);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}