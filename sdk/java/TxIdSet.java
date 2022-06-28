package io.accumulatenetwork.accumulate;

public class TxIdSet {
	public TxID[] entries;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.entries == null || this.entries.length == 0)) {
            data = Marshaller.writeTxid(data, 1, this.entries);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}