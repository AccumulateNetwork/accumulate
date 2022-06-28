package io.accumulatenetwork.accumulate;

public enum SignatureType {
    Unknown(0, "unknown"),
    LegacyED25519(1, "legacyED25519"),
    ED25519(2, "ed25519"),
    RCD1(3, "rcd1"),
    Receipt(4, "receipt"),
    Synthetic(5, "synthetic"),
    Set(6, "set"),
    Remote(7, "remote"),
    BTC(8, "btc"),
    BTCLegacy(9, "btclegacy"),
    ETH(10, "eth"),
    Delegated(11, "delegated"),
    Internal(12, "internal");

    final int value;
    final String name;
    SignatureType(int value, String name) {
        this.value = value;
        this.name = name;
    }

    public int getValue() { return this.value; }
    public String getName() { return this.name; }
    public String toString() { return this.name; }

    public static SignatureType byValue(int value) {
        switch (value) {
        case 0:
            return Unknown;
        case 1:
            return LegacyED25519;
        case 2:
            return ED25519;
        case 3:
            return RCD1;
        case 4:
            return Receipt;
        case 5:
            return Synthetic;
        case 6:
            return Set;
        case 7:
            return Remote;
        case 8:
            return BTC;
        case 9:
            return BTCLegacy;
        case 10:
            return ETH;
        case 11:
            return Delegated;
        case 12:
            return Internal;
        default:
            throw new RuntimeException(String.format("%d is not a valid SignatureType", value));
        }
    }

    public static SignatureType byName(String name) {
        switch (name.toLowerCase()) {
        case "unknown":
            return Unknown;
        case "legacyed25519":
            return LegacyED25519;
        case "ed25519":
            return ED25519;
        case "rcd1":
            return RCD1;
        case "receipt":
            return Receipt;
        case "synthetic":
            return Synthetic;
        case "set":
            return Set;
        case "remote":
            return Remote;
        case "btc":
            return BTC;
        case "btclegacy":
            return BTCLegacy;
        case "eth":
            return ETH;
        case "delegated":
            return Delegated;
        case "internal":
            return Internal;
        default:
            throw new RuntimeException(String.format("'%s' is not a valid SignatureType", name));
        }
    }
}