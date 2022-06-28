package io.accumulatenetwork.accumulate;

public class WriteDataResult {
	public byte[] entryHash;
	public Url accountUrl;
	public byte[] accountID;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.entryHash == null || this.entryHash.length == 0)) {
            data = Marshaller.writeHash(data, 2, this.entryHash);
        }
        if (!(this.accountUrl == null)) {
            data = Marshaller.writeUrl(data, 3, this.accountUrl);
        }
        if (!(this.accountID == null || this.accountID.length == 0)) {
            data = Marshaller.writeBytes(data, 4, this.accountID);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}