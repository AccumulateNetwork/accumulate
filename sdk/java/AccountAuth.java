package io.accumulatenetwork.accumulate;

public class AccountAuth {
	public AuthorityEntry[] authorities;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.authorities == null || this.authorities.length == 0)) {
            data = Marshaller.writeValue(data, 1, v.authorities);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}