package io.accumulatenetwork.accumulate;

public class NetworkGlobals {
	public Rational operatorAcceptThreshold;
	public string majorBlockSchedule;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.operatorAcceptThreshold == null)) {
            data = Marshaller.writeValue(data, 1, v.operatorAcceptThreshold);
        }
        if (!(this.majorBlockSchedule == null || this.majorBlockSchedule.length() == 0)) {
            data = Marshaller.writeString(data, 2, this.majorBlockSchedule);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}