package io.accumulatenetwork.accumulate;

public class SetThresholdKeyPageOperation {
	public int threshold;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.threshold == 0)) {
            data = Marshaller.writeUint(data, 2, this.threshold);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}