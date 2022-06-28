package io.accumulatenetwork.accumulate;

public class AuthorityEntry {
	public Url url;
	public bool disabled;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.url == null)) {
            data = Marshaller.writeUrl(data, 1, this.url);
        }
        if (!(!this.disabled)) {
            data = Marshaller.writeBool(data, 2, this.disabled);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}