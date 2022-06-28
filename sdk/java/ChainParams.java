package io.accumulatenetwork.accumulate;

public class ChainParams {
	public byte[] data;
	public bool isUpdate;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.data == null || this.data.length == 0)) {
            data = Marshaller.writeBytes(data, 1, this.data);
        }
        if (!(!this.isUpdate)) {
            data = Marshaller.writeBool(data, 2, this.isUpdate);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}