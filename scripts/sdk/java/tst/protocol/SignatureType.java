// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package io.accumulatenetwork.sdk.generated.protocol;

/**
    GENERATED BY go run ./tools/cmd/gen-api. DO NOT EDIT.
**/

import com.fasterxml.jackson.annotation.JsonValue;
import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.databind.JsonNode;
import io.accumulatenetwork.sdk.protocol.IntValueEnum;

public enum SignatureType implements IntValueEnum {
    (0, ""),
    (1, ""),
    (2, ""),
    (3, ""),
    (4, ""),
    (5, ""),
    (6, ""),
    (7, ""),
    (8, ""),
    (9, ""),
    (10, ""),
    (11, ""),
    (12, ""),
    (13, ""),
    (14, ""),
    (15, ""),
    (16, "");

    private final int value;
    private final String apiName;

    SignatureType(final int value, final String apiName) {
        this.value = value;
        this.apiName = apiName;
    }

    public int getValue() {
        return this.value;
    }

    @JsonValue
    public String getApiName() {
        return this.apiName;
    }

    public String toString() {
        return this.apiName;
    }

    public static SignatureType fromValue(final int value) {
        for (final var type : values()) {
            if (value == type.ordinal()) {
                return type;
            }
        }
        throw new RuntimeException(String.format("%d is not a valid TransactionType", value));
    }

    public static SignatureType fromApiName(final String name) {
        for (final var type : values()) {
            if (name != null && name.equalsIgnoreCase(type.apiName)) {
                return type;
            }
        }
        throw new RuntimeException(String.format("'%s' is not a valid TransactionType", name));
    }

    @JsonCreator
    public static SignatureType fromJsonNode(final JsonNode jsonNode) {
        for (final var type : values()) {
            if (jsonNode.isTextual() && jsonNode.asText().equalsIgnoreCase(type.apiName)) {
                return type;
            }
            if (jsonNode.isNumber() && jsonNode.asInt() == type.ordinal()) {
                return type;
            }
        }
        throw new RuntimeException(String.format("'%s' is not a valid TransactionType", jsonNode.toPrettyString()));
    }
}
