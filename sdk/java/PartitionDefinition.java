package io.accumulatenetwork.accumulate;

public class PartitionDefinition {
	public string partitionID;
	public byte[][] validatorKeys;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.partitionID == null || this.partitionID.length() == 0)) {
            data = Marshaller.writeString(data, 1, this.partitionID);
        }
        if (!(this.validatorKeys == null || this.validatorKeys.length == 0)) {
            data = Marshaller.writeBytes(data, 2, this.validatorKeys);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}