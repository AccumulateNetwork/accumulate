package io.accumulatenetwork.accumulate;

public class AddCredits {
	public Url recipient;
	public BigInteger amount;
	public int oracle;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.recipient == null)) {
            data = Marshaller.writeUrl(data, 2, this.recipient);
        }
        if (!((this.amount).equals(BigInteger.ZERO))) {
            data = Marshaller.writeBigInt(data, 3, this.amount);
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