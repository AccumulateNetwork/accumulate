package io.accumulatenetwork.accumulate;

public class RouteOverride {
	public Url account;
	public string partition;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.account == null)) {
            data = Marshaller.writeUrl(data, 1, this.account);
        }
        if (!(this.partition == null || this.partition.length() == 0)) {
            data = Marshaller.writeString(data, 2, this.partition);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}