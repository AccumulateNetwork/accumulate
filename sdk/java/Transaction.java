package io.accumulatenetwork.accumulate;

public class Transaction {
	public TransactionHeader header;
	public TransactionBody body;
	public byte[] hash;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.header == null)) {
            data = Marshaller.writeValue(data, 1, v.header);
        }
        if (!(this.body == nil)) {
            data = Marshaller.writeValue(data, 2, v.body);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}