package io.accumulatenetwork.accumulate;

public class CreateKeyBook {
	public Url url;
	public byte[] publicKeyHash;
	public Url[] authorities;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.url == null)) {
            data = Marshaller.writeUrl(data, 2, this.url);
        }
        if (!(this.publicKeyHash == null || this.publicKeyHash.length == 0)) {
            data = Marshaller.writeBytes(data, 3, this.publicKeyHash);
        }
        if (!(this.authorities == null || this.authorities.length == 0)) {
            data = Marshaller.writeUrl(data, 5, this.authorities);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}