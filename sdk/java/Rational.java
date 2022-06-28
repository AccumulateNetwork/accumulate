package io.accumulatenetwork.accumulate;

public class Rational {
	public int numerator;
	public int denominator;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.numerator == 0)) {
            data = Marshaller.writeUint(data, 1, this.numerator);
        }
        if (!(this.denominator == 0)) {
            data = Marshaller.writeUint(data, 2, this.denominator);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}