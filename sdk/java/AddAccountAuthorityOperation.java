package io.accumulatenetwork.accumulate;

public class AddAccountAuthorityOperation {
	public Url authority;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.authority == null)) {
            data = Marshaller.writeUrl(data, 2, this.authority);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}