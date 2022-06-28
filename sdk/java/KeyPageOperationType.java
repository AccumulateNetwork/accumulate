package io.accumulatenetwork.accumulate;

public enum KeyPageOperationType {
    Unknown(0, "unknown"),
    Update(1, "update"),
    Remove(2, "remove"),
    Add(3, "add"),
    SetThreshold(4, "setThreshold"),
    UpdateAllowed(5, "updateAllowed");

    final int value;
    final String name;
    KeyPageOperationType(int value, String name) {
        this.value = value;
        this.name = name;
    }

    public int getValue() { return this.value; }
    public String getName() { return this.name; }
    public String toString() { return this.name; }

    public static KeyPageOperationType byValue(int value) {
        switch (value) {
        case 0:
            return Unknown;
        case 1:
            return Update;
        case 2:
            return Remove;
        case 3:
            return Add;
        case 4:
            return SetThreshold;
        case 5:
            return UpdateAllowed;
        default:
            throw new RuntimeException(String.format("%d is not a valid KeyPageOperationType", value));
        }
    }

    public static KeyPageOperationType byName(String name) {
        switch (name.toLowerCase()) {
        case "unknown":
            return Unknown;
        case "update":
            return Update;
        case "remove":
            return Remove;
        case "add":
            return Add;
        case "setthreshold":
            return SetThreshold;
        case "updateallowed":
            return UpdateAllowed;
        default:
            throw new RuntimeException(String.format("'%s' is not a valid KeyPageOperationType", name));
        }
    }
}