package io.accumulatenetwork.accumulate;

public class CreateDataAccount {
	public Url url;
	public bool scratch;
	public Url[] authorities;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.url == null)) {
            data = Marshaller.writeUrl(data, 2, this.url);
        }
        if (!(!this.scratch)) {
            data = Marshaller.writeBool(data, 5, this.scratch);
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