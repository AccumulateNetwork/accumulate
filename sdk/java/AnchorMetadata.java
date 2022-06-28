package io.accumulatenetwork.accumulate;

public class AnchorMetadata {
	public ChainMetadata chainMetadata;
	public Url account;
	public int index;
	public int sourceIndex;
	public int sourceBlock;
	public byte[] entry;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.chainMetadata == null)) {
            data = Marshaller.writeValue(data, 1, v.chainMetadata);
        }
        if (!(this.account == null)) {
            data = Marshaller.writeUrl(data, 2, this.account);
        }
        if (!(this.index == 0)) {
            data = Marshaller.writeUint(data, 3, this.index);
        }
        if (!(this.sourceIndex == 0)) {
            data = Marshaller.writeUint(data, 4, this.sourceIndex);
        }
        if (!(this.sourceBlock == 0)) {
            data = Marshaller.writeUint(data, 5, this.sourceBlock);
        }
        if (!(this.entry == null || this.entry.length == 0)) {
            data = Marshaller.writeBytes(data, 6, this.entry);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}