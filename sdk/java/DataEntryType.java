package io.accumulatenetwork.accumulate;

public enum DataEntryType {
    Unknown(0, "unknown"),
    Factom(1, "factom"),
    Accumulate(2, "accumulate");

    final int value;
    final String name;
    DataEntryType(int value, String name) {
        this.value = value;
        this.name = name;
    }

    public int getValue() { return this.value; }
    public String getName() { return this.name; }
    public String toString() { return this.name; }

    public static DataEntryType byValue(int value) {
        switch (value) {
        case 0:
            return Unknown;
        case 1:
            return Factom;
        case 2:
            return Accumulate;
        default:
            throw new RuntimeException(String.format("%d is not a valid DataEntryType", value));
        }
    }

    public static DataEntryType byName(String name) {
        switch (name.toLowerCase()) {
        case "unknown":
            return Unknown;
        case "factom":
            return Factom;
        case "accumulate":
            return Accumulate;
        default:
            throw new RuntimeException(String.format("'%s' is not a valid DataEntryType", name));
        }
    }
}