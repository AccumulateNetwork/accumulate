package io.accumulatenetwork.accumulate;

public class ChainMetadata {
	public string name;
	public ChainType type;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.name == null || this.name.length() == 0)) {
            data = Marshaller.writeString(data, 1, this.name);
        }
        if (!(this.type == ChainType.Unknown)) {
            data = Marshaller.writeValue(data, 2, v.type);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}