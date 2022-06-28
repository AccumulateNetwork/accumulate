package io.accumulatenetwork.accumulate;

public class DelegatedSignature {
	public Signature signature;
	public Url delegator;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.signature == nil)) {
            data = Marshaller.writeValue(data, 2, v.signature);
        }
        if (!(this.delegator == null)) {
            data = Marshaller.writeUrl(data, 3, this.delegator);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}