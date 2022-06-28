package io.accumulatenetwork.accumulate;

public class TransactionStatus {
	public TxID txID;
	public errors2.Status code;
	public errors2.Error error;
	public TransactionResult result;
	public Url initiator;
	public Signer[] signers;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.txID == null)) {
            data = Marshaller.writeTxid(data, 1, this.txID);
        }
        if (!(this.code == errors2.Status.Unknown)) {
            data = Marshaller.writeValue(data, 2, v.code);
        }
        if (!(this.error == null)) {
            data = Marshaller.writeValue(data, 3, v.error);
        }
        if (!(this.result == nil)) {
            data = Marshaller.writeValue(data, 4, v.result);
        }
        if (!(this.initiator == null)) {
            data = Marshaller.writeUrl(data, 5, this.initiator);
        }
        if (!(this.signers == null || this.signers.length == 0)) {
            data = Marshaller.writeValue(data, 6, v.signers);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}