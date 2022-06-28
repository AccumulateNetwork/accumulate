package io.accumulatenetwork.accumulate;

public class TokenRecipient {
	public Url url;
	public BigInteger amount;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.url == null)) {
            data = Marshaller.writeUrl(data, 1, this.url);
        }
        if (!((this.amount).equals(BigInteger.ZERO))) {
            data = Marshaller.writeBigInt(data, 2, this.amount);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}