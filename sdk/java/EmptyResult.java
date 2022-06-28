package io.accumulatenetwork.accumulate;

public class EmptyResult {

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}