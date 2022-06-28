package io.accumulatenetwork.accumulate;

public class IndexEntry {
	public int source;
	public int anchor;
	public int blockIndex;
	public Time blockTime;
	public int rootIndexIndex;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.source == 0)) {
            data = Marshaller.writeUint(data, 1, this.source);
        }
        if (!(this.anchor == 0)) {
            data = Marshaller.writeUint(data, 2, this.anchor);
        }
        if (!(this.blockIndex == 0)) {
            data = Marshaller.writeUint(data, 3, this.blockIndex);
        }
        if (!(this.blockTime == null)) {
            data = Marshaller.writeTime(data, 4, this.blockTime);
        }
        if (!(this.rootIndexIndex == 0)) {
            data = Marshaller.writeUint(data, 5, this.rootIndexIndex);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}