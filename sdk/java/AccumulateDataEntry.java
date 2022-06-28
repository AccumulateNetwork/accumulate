package io.accumulatenetwork.accumulate;

public class AccumulateDataEntry {
	public byte[][] data;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.data == null || this.data.length == 0)) {
            data = Marshaller.writeBytes(data, 2, this.data);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}