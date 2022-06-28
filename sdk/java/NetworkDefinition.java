package io.accumulatenetwork.accumulate;

public class NetworkDefinition {
	public string networkName;
	public PartitionDefinition[] partitions;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.networkName == null || this.networkName.length() == 0)) {
            data = Marshaller.writeString(data, 1, this.networkName);
        }
        if (!(this.partitions == null || this.partitions.length == 0)) {
            data = Marshaller.writeValue(data, 2, v.partitions);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}