package io.accumulatenetwork.accumulate;

public class UpdateAllowedKeyPageOperation {
	public TransactionType[] allow;
	public TransactionType[] deny;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.allow == null || this.allow.length == 0)) {
            data = Marshaller.writeValue(data, 2, v.allow);
        }
        if (!(this.deny == null || this.deny.length == 0)) {
            data = Marshaller.writeValue(data, 3, v.deny);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}