package io.accumulatenetwork.accumulate;

public class UpdateAccountAuth {
	public AccountAuthOperation[] operations;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.operations == null || this.operations.length == 0)) {
            data = Marshaller.writeValue(data, 2, v.operations);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}