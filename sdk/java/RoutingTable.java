package io.accumulatenetwork.accumulate;

public class RoutingTable {
	public RouteOverride[] overrides;
	public Route[] routes;

    public byte[] marshalBinary() {
        byte[] data;
        if (!(this.overrides == null || this.overrides.length == 0)) {
            data = Marshaller.writeValue(data, 1, v.overrides);
        }
        if (!(this.routes == null || this.routes.length == 0)) {
            data = Marshaller.writeValue(data, 2, v.routes);
        }
        if (data == null || data.length == 0) {
            return { 0x80 };
        }
        return data;
    }
}