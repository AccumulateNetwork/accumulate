package io.accumulatenetwork.accumulate;

public class UpdateKeyPage {
	public KeyPageOperation[] operation;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.operation == null || this.operation.length == 0)) {
            data = Marshaller.writeValue(data, 2, v.operation);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}