package io.accumulatenetwork.accumulate;

public class TransactionHeader {
	public Url principal;
	public byte[] initiator;
	public string memo;
	public byte[] metadata;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.principal == null)) {
            data = Marshaller.writeUrl(data, 1, this.principal);
        }
        if (!(this.initiator == null || this.initiator.length == 0)) {
            data = Marshaller.writeHash(data, 2, this.initiator);
        }
        if (!(this.memo == null || this.memo.length() == 0)) {
            data = Marshaller.writeString(data, 3, this.memo);
        }
        if (!(this.metadata == null || this.metadata.length == 0)) {
            data = Marshaller.writeBytes(data, 4, this.metadata);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}