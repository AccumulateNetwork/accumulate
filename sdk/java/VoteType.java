package io.accumulatenetwork.accumulate;

public enum VoteType {
    Accept(0, "accept"),
    Reject(1, "reject"),
    Abstain(2, "abstain"),
    Suggest(3, "suggest");

    final int value;
    final String name;
    VoteType(int value, String name) {
        this.value = value;
        this.name = name;
    }

    public int getValue() { return this.value; }
    public String getName() { return this.name; }
    public String toString() { return this.name; }

    public static VoteType byValue(int value) {
        switch (value) {
        case 0:
            return Accept;
        case 1:
            return Reject;
        case 2:
            return Abstain;
        case 3:
            return Suggest;
        default:
            throw new RuntimeException(String.format("%d is not a valid VoteType", value));
        }
    }

    public static VoteType byName(String name) {
        switch (name.toLowerCase()) {
        case "accept":
            return Accept;
        case "reject":
            return Reject;
        case "abstain":
            return Abstain;
        case "suggest":
            return Suggest;
        default:
            throw new RuntimeException(String.format("'%s' is not a valid VoteType", name));
        }
    }
}