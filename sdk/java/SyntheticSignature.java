package io.accumulatenetwork.accumulate;

public class SyntheticSignature {
	public Url sourceNetwork;
	public Url destinationNetwork;
	public int sequenceNumber;
	public byte[] transactionHash;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.sourceNetwork == null)) {
            data = Marshaller.writeUrl(data, 2, this.sourceNetwork);
        }
        if (!(this.destinationNetwork == null)) {
            data = Marshaller.writeUrl(data, 3, this.destinationNetwork);
        }
        if (!(this.sequenceNumber == 0)) {
            data = Marshaller.writeUint(data, 4, this.sequenceNumber);
        }
        if (!(this.transactionHash == null || this.transactionHash.length == 0)) {
            data = Marshaller.writeHash(data, 5, this.transactionHash);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}