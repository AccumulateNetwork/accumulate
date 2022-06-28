package io.accumulatenetwork.accumulate;

public class IssueTokens {
	public Url recipient;
	public BigInteger amount;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.recipient == null)) {
            data = Marshaller.writeUrl(data, 2, this.recipient);
        }
        if (!((this.amount).equals(BigInteger.ZERO))) {
            data = Marshaller.writeBigInt(data, 3, this.amount);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}