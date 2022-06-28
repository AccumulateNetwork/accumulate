package io.accumulatenetwork.accumulate;

public class Route {
	public int length;
	public int value;
	public string partition;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.length == 0)) {
            data = Marshaller.writeUint(data, 1, this.length);
        }
        if (!(this.value == 0)) {
            data = Marshaller.writeUint(data, 2, this.value);
        }
        if (!(this.partition == null || this.partition.length() == 0)) {
            data = Marshaller.writeString(data, 3, this.partition);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}