package io.accumulatenetwork.accumulate;

public class AddCreditsResult {
	public BigInteger amount;
	public int credits;
	public int oracle;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!((this.amount).equals(BigInteger.ZERO))) {
            data = Marshaller.writeBigInt(data, 2, this.amount);
        }
        if (!(this.credits == 0)) {
            data = Marshaller.writeUint(data, 3, this.credits);
        }
        if (!(this.oracle == 0)) {
            data = Marshaller.writeUint(data, 4, this.oracle);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}