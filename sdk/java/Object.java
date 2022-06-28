package io.accumulatenetwork.accumulate;

public class Object {
	public ObjectType type;
	public ChainMetadata[] chains;
	public TxIdSet pending;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.type == ObjectType.Unknown)) {
            data = Marshaller.writeValue(data, 1, v.type);
        }
        if (!(this.chains == null || this.chains.length == 0)) {
            data = Marshaller.writeValue(data, 2, v.chains);
        }
        if (!(this.pending == null)) {
            data = Marshaller.writeValue(data, 3, v.pending);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}