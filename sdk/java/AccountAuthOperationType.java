package io.accumulatenetwork.accumulate;

public enum AccountAuthOperationType {
    Unknown(0, "unknown"),
    Enable(1, "enable"),
    Disable(2, "disable"),
    AddAuthority(3, "addAuthority"),
    RemoveAuthority(4, "removeAuthority");

    final int value;
    final String name;
    AccountAuthOperationType(int value, String name) {
        this.value = value;
        this.name = name;
    }

    public int getValue() { return this.value; }
    public String getName() { return this.name; }
    public String toString() { return this.name; }

    public static AccountAuthOperationType byValue(int value) {
        switch (value) {
        case 0:
            return Unknown;
        case 1:
            return Enable;
        case 2:
            return Disable;
        case 3:
            return AddAuthority;
        case 4:
            return RemoveAuthority;
        default:
            throw new RuntimeException(String.format("%d is not a valid AccountAuthOperationType", value));
        }
    }

    public static AccountAuthOperationType byName(String name) {
        switch (name.toLowerCase()) {
        case "unknown":
            return Unknown;
        case "enable":
            return Enable;
        case "disable":
            return Disable;
        case "addauthority":
            return AddAuthority;
        case "removeauthority":
            return RemoveAuthority;
        default:
            throw new RuntimeException(String.format("'%s' is not a valid AccountAuthOperationType", name));
        }
    }
}