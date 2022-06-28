package io.accumulatenetwork.accumulate;

public enum TransactionType {
    Unknown(0, "unknown"),
    CreateIdentity(1, "createIdentity"),
    CreateTokenAccount(2, "createTokenAccount"),
    SendTokens(3, "sendTokens"),
    CreateDataAccount(4, "createDataAccount"),
    WriteData(5, "writeData"),
    WriteDataTo(6, "writeDataTo"),
    AcmeFaucet(7, "acmeFaucet"),
    CreateToken(8, "createToken"),
    IssueTokens(9, "issueTokens"),
    BurnTokens(10, "burnTokens"),
    CreateKeyPage(12, "createKeyPage"),
    CreateKeyBook(13, "createKeyBook"),
    AddCredits(14, "addCredits"),
    UpdateKeyPage(15, "updateKeyPage"),
    UpdateAccountAuth(21, "updateAccountAuth"),
    UpdateKey(22, "updateKey"),
    Remote(48, "remote"),
    SyntheticCreateIdentity(49, "syntheticCreateIdentity"),
    SyntheticWriteData(50, "syntheticWriteData"),
    SyntheticDepositTokens(51, "syntheticDepositTokens"),
    SyntheticDepositCredits(52, "syntheticDepositCredits"),
    SyntheticBurnTokens(53, "syntheticBurnTokens"),
    SyntheticForwardTransaction(54, "syntheticForwardTransaction"),
    SystemGenesis(96, "systemGenesis"),
    DirectoryAnchor(97, "directoryAnchor"),
    BlockValidatorAnchor(98, "blockValidatorAnchor"),
    SystemWriteData(99, "systemWriteData");

    final int value;
    final String name;
    TransactionType(int value, String name) {
        this.value = value;
        this.name = name;
    }

    public int getValue() { return this.value; }
    public String getName() { return this.name; }
    public String toString() { return this.name; }

    public static TransactionType byValue(int value) {
        switch (value) {
        case 0:
            return Unknown;
        case 1:
            return CreateIdentity;
        case 2:
            return CreateTokenAccount;
        case 3:
            return SendTokens;
        case 4:
            return CreateDataAccount;
        case 5:
            return WriteData;
        case 6:
            return WriteDataTo;
        case 7:
            return AcmeFaucet;
        case 8:
            return CreateToken;
        case 9:
            return IssueTokens;
        case 10:
            return BurnTokens;
        case 12:
            return CreateKeyPage;
        case 13:
            return CreateKeyBook;
        case 14:
            return AddCredits;
        case 15:
            return UpdateKeyPage;
        case 21:
            return UpdateAccountAuth;
        case 22:
            return UpdateKey;
        case 48:
            return Remote;
        case 49:
            return SyntheticCreateIdentity;
        case 50:
            return SyntheticWriteData;
        case 51:
            return SyntheticDepositTokens;
        case 52:
            return SyntheticDepositCredits;
        case 53:
            return SyntheticBurnTokens;
        case 54:
            return SyntheticForwardTransaction;
        case 96:
            return SystemGenesis;
        case 97:
            return DirectoryAnchor;
        case 98:
            return BlockValidatorAnchor;
        case 99:
            return SystemWriteData;
        default:
            throw new RuntimeException(String.format("%d is not a valid TransactionType", value));
        }
    }

    public static TransactionType byName(String name) {
        switch (name.toLowerCase()) {
        case "unknown":
            return Unknown;
        case "createidentity":
            return CreateIdentity;
        case "createtokenaccount":
            return CreateTokenAccount;
        case "sendtokens":
            return SendTokens;
        case "createdataaccount":
            return CreateDataAccount;
        case "writedata":
            return WriteData;
        case "writedatato":
            return WriteDataTo;
        case "acmefaucet":
            return AcmeFaucet;
        case "createtoken":
            return CreateToken;
        case "issuetokens":
            return IssueTokens;
        case "burntokens":
            return BurnTokens;
        case "createkeypage":
            return CreateKeyPage;
        case "createkeybook":
            return CreateKeyBook;
        case "addcredits":
            return AddCredits;
        case "updatekeypage":
            return UpdateKeyPage;
        case "updateaccountauth":
            return UpdateAccountAuth;
        case "updatekey":
            return UpdateKey;
        case "remote":
            return Remote;
        case "signPending":
            return Remote;
        case "syntheticcreateidentity":
            return SyntheticCreateIdentity;
        case "syntheticwritedata":
            return SyntheticWriteData;
        case "syntheticdeposittokens":
            return SyntheticDepositTokens;
        case "syntheticdepositcredits":
            return SyntheticDepositCredits;
        case "syntheticburntokens":
            return SyntheticBurnTokens;
        case "syntheticforwardtransaction":
            return SyntheticForwardTransaction;
        case "systemgenesis":
            return SystemGenesis;
        case "directoryanchor":
            return DirectoryAnchor;
        case "blockvalidatoranchor":
            return BlockValidatorAnchor;
        case "systemwritedata":
            return SystemWriteData;
        default:
            throw new RuntimeException(String.format("'%s' is not a valid TransactionType", name));
        }
    }
}