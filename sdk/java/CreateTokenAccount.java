package io.accumulatenetwork.accumulate;

public class CreateTokenAccount {
	public Url url;
	public Url tokenUrl;
	public bool scratch;
	public Url[] authorities;
	public AccountStateProof tokenIssuerProof;

    public byte[] marshalBinary() {
        byte[] data;
        data = Marshaller.writeValue(data, 1, v.type);
        if (!(this.url == null)) {
            data = Marshaller.writeUrl(data, 2, this.url);
        }
        if (!(this.tokenUrl == null)) {
            data = Marshaller.writeUrl(data, 3, this.tokenUrl);
        }
        if (!(!this.scratch)) {
            data = Marshaller.writeBool(data, 5, this.scratch);
        }
        if (!(this.authorities == null || this.authorities.length == 0)) {
            data = Marshaller.writeUrl(data, 7, this.authorities);
        }
        if (!(this.tokenIssuerProof == null)) {
            data = Marshaller.writeValue(data, 8, v.tokenIssuerProof);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}