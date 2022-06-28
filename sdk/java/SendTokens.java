package io.accumulatenetwork.accumulate;

public class SendTokens {
	public byte[] hash;
	public RawJson meta;
	public TokenRecipient[] to;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.hash == null || this.hash.length == 0)) {
            data = Marshaller.writeHash(data, 2, this.hash);
        }
        if (!(this.meta == null || this.meta.length == 0)) {
            data = Marshaller.writeRawJson(data, 3, this.meta);
        }
        if (!(this.to == null || this.to.length == 0)) {
            data = Marshaller.writeValue(data, 4, v.to);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}