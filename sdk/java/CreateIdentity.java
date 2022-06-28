package io.accumulatenetwork.accumulate;

public class CreateIdentity {
	public Url url;
	public byte[] keyHash;
	public Url keyBookUrl;
	public Url[] authorities;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.url == null)) {
            data = Marshaller.writeUrl(data, 2, this.url);
        }
        if (!(this.keyHash == null || this.keyHash.length == 0)) {
            data = Marshaller.writeBytes(data, 3, this.keyHash);
        }
        if (!(this.keyBookUrl == null)) {
            data = Marshaller.writeUrl(data, 4, this.keyBookUrl);
        }
        if (!(this.authorities == null || this.authorities.length == 0)) {
            data = Marshaller.writeUrl(data, 6, this.authorities);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}