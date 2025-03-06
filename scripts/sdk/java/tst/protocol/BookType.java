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

public enum BookType implements IntValueEnum {
    (0, ""),
    (1, ""),
    (2, "");

    private final int value;
    private final String apiName;

    BookType(final int value, final String apiName) {
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

    public static BookType fromValue(final int value) {
        for (final var type : values()) {
            if (value == type.ordinal()) {
                return type;
            }
        }
        throw new RuntimeException(String.format("%d is not a valid TransactionType", value));
    }

    public static BookType fromApiName(final String name) {
        for (final var type : values()) {
            if (name != null && name.equalsIgnoreCase(type.apiName)) {
                return type;
            }
        }
        throw new RuntimeException(String.format("'%s' is not a valid TransactionType", name));
    }

    @JsonCreator
    public static BookType fromJsonNode(final JsonNode jsonNode) {
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
