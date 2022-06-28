package io.accumulatenetwork.accumulate;

public class AccountStateProof {
	public Account state;
	public managed.Receipt proof;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.state == nil)) {
            data = Marshaller.writeValue(data, 1, v.state);
        }
        if (!(this.proof == null)) {
            data = Marshaller.writeValue(data, 2, v.proof);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}