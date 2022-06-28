package io.accumulatenetwork.accumulate;

public class RemoteSignature {
	public Url destination;
	public Signature signature;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.destination == null)) {
            data = Marshaller.writeUrl(data, 2, this.destination);
        }
        if (!(this.signature == nil)) {
            data = Marshaller.writeValue(data, 3, v.signature);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}