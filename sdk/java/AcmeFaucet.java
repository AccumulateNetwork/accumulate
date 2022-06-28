package io.accumulatenetwork.accumulate;

public class AcmeFaucet {
	public Url url;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.url == null)) {
            data = Marshaller.writeUrl(data, 2, this.url);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}