package io.accumulatenetwork.accumulate;

public class CreateToken {
	public Url url;
	public string symbol;
	public int precision;
	public Url properties;
	public BigInteger supplyLimit;
	public Url[] authorities;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.url == null)) {
            data = Marshaller.writeUrl(data, 2, this.url);
        }
        if (!(this.symbol == null || this.symbol.length() == 0)) {
            data = Marshaller.writeString(data, 4, this.symbol);
        }
        if (!(this.precision == 0)) {
            data = Marshaller.writeUint(data, 5, this.precision);
        }
        if (!(this.properties == null)) {
            data = Marshaller.writeUrl(data, 6, this.properties);
        }
        if (!((this.supplyLimit).equals(BigInteger.ZERO))) {
            data = Marshaller.writeBigInt(data, 7, this.supplyLimit);
        }
        if (!(this.authorities == null || this.authorities.length == 0)) {
            data = Marshaller.writeUrl(data, 9, this.authorities);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}