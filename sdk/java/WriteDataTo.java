package io.accumulatenetwork.accumulate;

public class WriteDataTo {
	public Url recipient;
	public DataEntry entry;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.recipient == null)) {
            data = Marshaller.writeUrl(data, 2, this.recipient);
        }
        if (!(this.entry == nil)) {
            data = Marshaller.writeValue(data, 3, v.entry);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}