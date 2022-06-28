package io.accumulatenetwork.accumulate;

public class BurnTokens {
	public BigInteger amount;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!((this.amount).equals(BigInteger.ZERO))) {
            data = Marshaller.writeBigInt(data, 2, this.amount);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}