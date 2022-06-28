package io.accumulatenetwork.accumulate;

public class AcmeOracle {
	public int price;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.price == 0)) {
            data = Marshaller.writeUint(data, 1, this.price);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}