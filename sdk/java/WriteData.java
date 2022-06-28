package io.accumulatenetwork.accumulate;

public class WriteData {
	public DataEntry entry;
	public bool scratch;
	public bool writeToState;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.entry == nil)) {
            data = Marshaller.writeValue(data, 2, v.entry);
        }
        if (!(!this.scratch)) {
            data = Marshaller.writeBool(data, 3, this.scratch);
        }
        if (!(!this.writeToState)) {
            data = Marshaller.writeBool(data, 4, this.writeToState);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}