package io.accumulatenetwork.accumulate;

public class FactomDataEntry {
	public byte[] accountId;
	public byte[] data;
	public byte[][] extIds;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.accountId == null || this.accountId.length == 0)) {
            data = Marshaller.writeHash(data, 2, this.accountId);
        }
        if (!(this.data == null || this.data.length == 0)) {
            data = Marshaller.writeBytes(data, 3, this.data);
        }
        if (!(this.extIds == null || this.extIds.length == 0)) {
            data = Marshaller.writeBytes(data, 4, this.extIds);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}